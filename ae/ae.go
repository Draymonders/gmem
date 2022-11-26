package ae

import (
	"fmt"
	"log"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type FileEventType int

const (
	FileEventType_Readable  FileEventType = 1 // 可读
	FileEventType_Writeable FileEventType = 2 // 可写
)

var fileEventToPoll = [3]uint32{0, unix.POLLIN, unix.POLLOUT}

type FileProcFn func(extra interface{}) // 文件处理回调
type TimeProcFn func(extra interface{}) // 时间处理回调

// FileEvent 文件事件
type FileEvent struct {
	fd            int
	fileEventType FileEventType
	fileFn        FileProcFn
	extra         interface{} // 客户端信息, maybe Client
}

type TimeEventType int

const (
	TimeEventType_Cycle TimeEventType = 1 // 循环
	TimeEventType_Once  TimeEventType = 2 // 一次性
)

// TimeEvent 时间事件
type TimeEvent struct {
	id            int   // time event id
	when          int64 // 下次定时任务执行时间点
	interval      int64 // 时间间隔
	timeFn        TimeProcFn
	timeEventType TimeEventType
	extra         interface{}
	next          *TimeEvent // 下一个时间事件，链表目前非有序，时间事件不多，多的话，可以进行优化
}

type EventLoop struct {
	fileEvents      map[int]*FileEvent // (fd + feType) -> fileEvent
	timeEvents      *TimeEvent
	fileEventFd     int // server epoll fd
	timeEventNextId int
	stop            bool
}

func CreateEventLoop() (loop *EventLoop, err error) {
	var fd int
	fd, err = unix.EpollCreate(1)
	if err != nil {
		log.Printf("EpollCreate err: %v", err)
		return nil, err
	}
	loop = &EventLoop{
		fileEvents:      make(map[int]*FileEvent),
		fileEventFd:     fd,
		timeEventNextId: 0,
		stop:            false,
	}
	return
}

func getFdMask(fd int, eventType FileEventType) int {
	if eventType == FileEventType_Readable {
		return fd
	} else if eventType == FileEventType_Writeable {
		return fd * -1
	}
	panic(fmt.Sprintf("not support eventType %v", eventType))
}

// 获取当前fd 在eventLoop里面的新的 poll类型
func (loop *EventLoop) getFdPollType(fd int) uint32 {
	v := uint32(0)
	if _, ok := loop.fileEvents[getFdMask(fd, FileEventType_Readable)]; ok {
		v |= fileEventToPoll[FileEventType_Readable]
	}
	if _, ok := loop.fileEvents[getFdMask(fd, FileEventType_Writeable)]; ok {
		v |= fileEventToPoll[FileEventType_Writeable]
	}
	return v
}

func (loop *EventLoop) AddEvent(fd int, eventType FileEventType, fn FileProcFn, extra interface{}) (err error) {
	epollCtl := syscall.EPOLL_CTL_ADD
	curPollEvents := loop.getFdPollType(fd)
	if curPollEvents != 0 {
		epollCtl = syscall.EPOLL_CTL_MOD
	}
	curPollEvents |= fileEventToPoll[eventType]
	err = unix.EpollCtl(loop.fileEventFd, epollCtl, fd, &unix.EpollEvent{Events: curPollEvents, Fd: int32(fd)})
	if err != nil {
		return err
	}
	loop.fileEvents[getFdMask(fd, eventType)] = &FileEvent{
		fd:            fd,
		fileEventType: eventType,
		fileFn:        fn,
		extra:         extra,
	}
	return nil
}

func (loop *EventLoop) DelEvent(fd int, eventType FileEventType) (err error) {
	epollCtl := syscall.EPOLL_CTL_DEL
	curPollEvents := loop.getFdPollType(fd)

	curPollEvents &= ^fileEventToPoll[eventType]
	if curPollEvents != 0 {
		epollCtl = syscall.EPOLL_CTL_MOD
	}
	err = unix.EpollCtl(loop.fileEventFd, epollCtl, fd, &unix.EpollEvent{Events: curPollEvents, Fd: int32(fd)})
	if err != nil {
		return err
	}
	delete(loop.fileEvents, getFdMask(fd, eventType))
	return nil
}

func (loop *EventLoop) AddTimeEvent(interval int64, eventType TimeEventType, fn TimeProcFn, extra interface{}) {
	loop.timeEventNextId++
	id := loop.timeEventNextId

	if interval <= 0 {
		interval = 10
	}

	loop.timeEvents = &TimeEvent{
		id:            id,
		timeEventType: eventType,
		when:          GetUnixTime() + interval,
		interval:      interval,
		timeFn:        fn,
		next:          loop.timeEvents,
		extra:         extra,
	}
	return
}

func (loop *EventLoop) DelTimeEvent(id int) {
	var pre *TimeEvent
	cur := loop.timeEvents
	for cur != nil {
		if cur.id == id {
			if pre == nil {
				loop.timeEvents = cur
			} else {
				pre.next = cur.next
			}
			cur.next = nil
			break
		}
		pre = cur
		cur = cur.next
	}
}

// 测试使用
func (loop *EventLoop) Range() (n int) {
	log.Printf("====")
	for cur := loop.timeEvents; cur != nil; cur = cur.next {
		n++
		log.Printf("timeEventId: %v", cur.id)
	}
	log.Printf("====")
	return
}

func (loop *EventLoop) Wait() ([]*FileEvent, []*TimeEvent, error) {
	var (
		fes = make([]*FileEvent, 0)
		tes = make([]*TimeEvent, 0)
		err error
	)

	waitMs := 0
	curMs := GetUnixTime()

	if earliestTimer := loop.getEarliestTimer(); earliestTimer != nil {
		if earliestTimer.when > curMs {
			waitMs = int(earliestTimer.when - curMs)
		}
		for cur := loop.timeEvents; cur != nil; cur = cur.next {
			if cur.when <= curMs {
				tes = append(tes, cur)
			}
		}
	}
	if waitMs <= 0 {
		waitMs = 10
	}

	events := make([]unix.EpollEvent, 64)
retry:
	n, err := unix.EpollWait(loop.fileEventFd, events, waitMs)
	if err != nil {
		if err == unix.EINTR {
			goto retry
		}
		log.Printf("EpollWait err: %v", err)
		return nil, nil, err
	}

	for i := 0; i < n; i++ {
		fd := int(events[i].Fd)
		if events[i].Events&unix.POLLIN != 0 {
			fes = append(fes, loop.fileEvents[getFdMask(fd, FileEventType_Readable)])
		}
		if events[i].Events&unix.POLLOUT != 0 {
			fes = append(fes, loop.fileEvents[getFdMask(fd, FileEventType_Writeable)])
		}
	}
	return fes, tes, nil
}

func (loop *EventLoop) AeMain() error {
	for loop.stop == false {
		fes, tes, err := loop.Wait()
		if err != nil {
			log.Printf("loop.Wait err: %v", err)
			loop.stop = true // 把获取到的事件，都先执行完，再关闭 时间循环
		}

		curMs := GetUnixTime()
		for _, te := range tes {
			te.timeFn(te.extra)
			if te.timeEventType == TimeEventType_Once {
				loop.DelTimeEvent(te.id)
			} else if te.timeEventType == TimeEventType_Cycle {
				te.when = curMs + te.interval
			}
		}

		for _, fileEvent := range fes {
			fileEvent.fileFn(fileEvent.extra)
		}
	}
	return nil
}

// GetUnixTime 单位为ms
func GetUnixTime() int64 {
	return time.Now().UnixMilli()
}

func (loop *EventLoop) getEarliestTimer() *TimeEvent {
	var earliestTimer *TimeEvent

	for cur := loop.timeEvents; cur != nil; cur = cur.next {
		if earliestTimer == nil {
			earliestTimer = cur
		} else if cur.when < earliestTimer.when {
			earliestTimer = cur
		}
	}
	return earliestTimer
}

/*
func socketFD(conn net.Conn) int {
	//tls := reflect.TypeOf(conn.UnderlyingConn()) == reflect.TypeOf(&tls.Conn{})
	// Extract the file descriptor associated with the connection
	//connVal := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn").Elem()
	tcpConn := reflect.Indirect(reflect.ValueOf(conn)).FieldByName("conn")
	//if tls {
	//	tcpConn = reflect.Indirect(tcpConn.Elem())
	//}
	fdVal := tcpConn.FieldByName("fd")
	pfdVal := reflect.Indirect(fdVal).FieldByName("pfd")

	v1 := int(pfdVal.FieldByName("Sysfd").Int())

	f, _ := conn.(*net.TCPConn).File()
	v2 := f.Fd()

	if v1 != int(v2) {
		panic(fmt.Sprintf("fd v1 %v v2 %v", v1, v2))
	}
	return v1
}

func (loop *EventLoop) AddEventByConn(conn net.Conn, eventType FileEventType, fn FileProcFn, extra interface{}) (err error) {
	fd := socketFD(conn)
	return loop.AddEvent(fd, eventType, fn, extra)
}

func (loop *EventLoop) DelEventByConn(conn net.Conn, eventType FileEventType) (err error) {
	fd := socketFD(conn)
	return loop.DelEvent(fd, eventType)
}
*/
