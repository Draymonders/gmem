package main

import (
	"net"
	"strconv"
	"testing"

	"github.com/draymonders/gmem/ae"
)

//func Test_Epoll(t *testing.T) {
//	efd, err := unix.EpollCreate1(0) // no CLOEXEC equivalent on z/OS
//	if err != nil {
//		t.Fatalf("EpollCreate1: %v", err)
//	}
//	// no need to defer a close on efd, as it's not a real file descriptor on zos
//
//	r, w, err := os.Pipe()
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer r.Close()
//	defer w.Close()
//
//	fd := int(r.Fd())
//	ev := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(fd)}
//
//	err = unix.EpollCtl(efd, unix.EPOLL_CTL_ADD, fd, &ev)
//	if err != nil {
//		t.Fatalf("EpollCtl: %v", err)
//	}
//
//	if _, err := w.Write([]byte("HELLO GOPHER")); err != nil {
//		t.Fatal(err)
//	}
//
//	events := make([]unix.EpollEvent, 128)
//	n, err := unix.EpollWait(efd, events, 1)
//	if err != nil {
//		t.Fatalf("EpollWait: %v", err)
//	}
//
//	if n != 1 {
//		t.Errorf("EpollWait: wrong number of events: got %v, expected 1", n)
//	}
//
//	got := int(events[0].Fd)
//	if got != fd {
//		t.Errorf("EpollWait: wrong Fd in event: got %v, expected %v", got, fd)
//	}
//
//	if events[0].Events&unix.EPOLLIN == 0 {
//		t.Errorf("Expected EPOLLIN flag to be set, got %b", events[0].Events)
//	}
//
//}

func Test_TcpNet(t *testing.T) {
	ln, err := net.Listen("127.0.0.1", ":8081")
	if err != nil {
		t.FailNow()
	}
	t.Logf("%+v\n", ln)
}

func Test_TimeEvent(t *testing.T) {
	// 时间事件创建 & 销毁测试
	loop := &ae.EventLoop{}
	var teType = ae.TimeEventType_Cycle
	n := 3
	for i := 0; i < n; i++ {
		loop.AddTimeEvent(10, teType, nil, nil)
	}
	loop.DelTimeEvent(1)
	loop.Range() // size must be 2, and id in (3, 2)

	loop.AddTimeEvent(10, teType, nil, nil)
	loop.Range() // size must be 3, and id in (4, 3, 2)

	loop.DelTimeEvent(3)
	loop.Range() // size must be 2, and id in (4, 2)

}

func Test_BulkQuery(t *testing.T) {
	queryBuf := []byte("*2\r\n$5\r\nhello\r\n$5\r\nworld\r\n")

	c := &Client{queryBuf: queryBuf, queryLen: len(queryBuf), args: make([]*Obj, 0)}

	if ok, err := handleBulkQueryStream(c); !ok || err != nil {
		t.Logf("ok: %v err: %v", ok, err)
		t.FailNow()
	}
	wantArgs := []string{"hello", "world"}
	for i, arg := range c.args {
		if arg.ToStr() != wantArgs[i] {
			t.Logf("args[%d] want %v, but cur %v", i, wantArgs[i], arg.ToStr())
			t.FailNow()
		}
	}
}

func Test_Dict(t *testing.T) {
	dict := NewDict(DictType{HashFn: Hash, EqualFn: Equal})

	if v := dict.Get(NewObjectFromStr("k1")); v != nil {
		t.Logf("get key k1 expect null, but v is not null")
		t.FailNow()
	}

	if err := dict.Set(NewObjectFromStr("k1"), NewObjectFromStr("v1")); err != errorNotFound {
		t.Logf("set key k1 expect err, but err is %v", err)
		t.FailNow()
	}

	if err := dict.Add(NewObjectFromStr("k1"), NewObjectFromStr("v1")); err != nil {
		t.Logf("set key k1 expect success, but err is %v", err)
		t.FailNow()
	}

	if err := dict.Add(NewObjectFromStr("k1"), NewObjectFromStr("v1")); err == nil {
		t.Logf("add key k1 expect err, but err is %v", err)
		t.FailNow()
	}

	if err := dict.Set(NewObjectFromStr("k1"), NewObjectFromStr("v2")); err != nil {
		t.Logf("set key k1 expect success, but err is %v", err)
		t.FailNow()
	}

	if v := dict.Get(NewObjectFromStr("k1")); v == nil || v.ToStr() != "v2" {
		t.Logf("get key k1 expect v2, but v is %+v", v)
		t.FailNow()
	}
}

func Test_DictExpand(t *testing.T) {
	n := 10000
	dict := NewDict(DictType{HashFn: Hash, EqualFn: Equal})

	for i := 1; i <= n; i++ {
		v := strconv.FormatInt(int64(i), 10)
		_ = dict.Add(NewObjectFromStr(v), NewObjectFromStr(v))
	}

	for dict.isRehash() {
		dict.expandStep()
	}

	t.Logf("used %v", dict.ht[0].used)
	if dict.ht[0].used != n {
		t.Logf("used expect n: %v", n)
		t.FailNow()
	}

	if entry := dict.RandomGet(); entry == nil {
		t.Logf("expect entry is not nil")
		t.FailNow()
	} else {
		t.Logf("entry key: %v val: %v", entry.key.ToStr(), entry.val.ToStr())
	}
}
