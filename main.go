package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/draymonders/gmem/ae"
	"github.com/draymonders/gmem/conf"
)

const (
	MaxClientQueryBufferLen = 1024 * 4 // 4KB
)

type cmdType int // 请求Command类型
const (
	cmdType_Unknown cmdType = 0
	cmdType_Inline  cmdType = 1
	cmdType_Bulk    cmdType = 2
)

var server Server

type Server struct {
	port int
	fd   int // server Fd

	eventLoop *ae.EventLoop   // aeLoop
	clients   map[int]*Client // fd -> client
	db        *DB             // storage
}

type Client struct {
	fd int // client Fd
	db *DB

	bulkNum int // bulk query strings num
	bulkLen int // single string query length

	queryBuf []byte // queryBuf -> args
	queryLen int
	sentLen  int // cache sentBuf len
	cmdType  cmdType

	args  []*Obj // args -> reply
	reply *List
}

type DB struct {
	expires *Dict // key是否过期
	dict    *Dict // key -> gObj
}

func main() {
	path := os.Args[1]
	var (
		cf  *conf.Config
		err error
	)
	if cf, err = conf.LoadConf(path); err != nil {
		log.Printf("load conf err: %v", err)
		return
	}
	if err = initServer(cf); err != nil {
		log.Printf("init Server err: %v", err)
		return
	}
	log.Printf("init Server Success, port: %v", server.port)
	if err = server.eventLoop.AeMain(); err != nil {
		log.Printf("AeMain err: %v", err)
		return
	}
}

func initServer(cf *conf.Config) (err error) {
	server.port = cf.Port
	// 1. 初始化server数据结构
	server.clients = make(map[int]*Client)
	server.db = &DB{
		expires: NewDict(DictType{HashFn: Hash, EqualFn: Equal}),
		dict:    NewDict(DictType{HashFn: Hash, EqualFn: Equal}),
	}
	// 2. 建立tcp链接，获取fd
	if server.fd, err = TcpServer(server.port); err != nil {
		log.Printf("TcpServer err: %v", err)
		return err
	}
	// 3. 创建 AeEventLoop
	if server.eventLoop, err = ae.CreateEventLoop(); err != nil {
		log.Printf("createEventLoop err: %v", err)
		return err
	}
	// 3.1 监听文件事件循环
	if err = server.eventLoop.AddEvent(server.fd, ae.FileEventType_Readable, acceptHandler, nil); err != nil {
		return err
	}
	// 3.2 监听时间事件循环
	server.eventLoop.AddTimeEvent(10, ae.TimeEventType_Cycle, serverCron, nil)
	return nil
}

// 定时清理过期key
func serverCron(extra interface{}) {
	const scanSize = 1
	unixTs := time.Now().Unix()
	for idx := 0; idx < scanSize; idx++ {
		if entry := server.db.expires.RandomGet(); entry != nil {
			expireTs, err := entry.val.ToInt64()
			if err != nil || expireTs == -1 {
				continue
			}
			if unixTs >= expireTs {
				server.db.expires.Del(entry.key)
				server.db.dict.Del(entry.key)
			}
		}
	}

}

func acceptHandler(extra interface{}) {
	cfd, err := Accept(server.fd)
	if err != nil {
		log.Printf("acceptHandler extra %+v not Client", extra)
		return
	}
	log.Printf("client fd: %v accept", cfd)

	client := &Client{
		fd:       cfd, // client default db
		db:       server.db,
		queryBuf: make([]byte, 0),
		args:     make([]*Obj, 0),
		reply:    NewList(ListType{EqualFn: ListEqualFn}),
	}
	server.clients[cfd] = client

	if err = server.eventLoop.AddEvent(cfd, ae.FileEventType_Readable, readQueryFromClient, client); err != nil {
		log.Printf("cfd: %d add event readQueryFromClient err: %+v", cfd, err)
		freeClient(client)
	}
}

func readQueryFromClient(extra interface{}) {
	c, ok := extra.(*Client)
	if !ok || c == nil {
		log.Printf("readQueryFromClient extra %+v not Client", extra)
		return
	}
	log.Printf("client fd %v readQueryFromClient", c.fd)
	var (
		n   int
		err error
	)
	if len(c.queryBuf)-c.queryLen < MaxClientQueryBufferLen {
		c.queryBuf = append(c.queryBuf, make([]byte, MaxClientQueryBufferLen)...)
	}
	if n, err = Read(c.fd, c.queryBuf[c.queryLen:]); err != nil {
		log.Printf("read from client fd %v err: %v", c.fd, err)
		freeClient(c)
		return
	}
	c.queryLen += n
	if err = processInputBuffer(c); err != nil {
		log.Printf("client fd %v processInputBuffer err: %v", c.fd, err)
		freeClient(c)
		return
	}
}

func processInputBuffer(c *Client) (err error) {
	// 解析命令，将 queryBuf -> c.args
	for c.queryLen > 0 { // 先处理下bulk
		c.cmdType = parseCmdType(c)
		var ok bool
		switch c.cmdType {
		case cmdType_Bulk:
			ok, err = handleBulkQueryStream(c)
		case cmdType_Inline:
			ok, err = handleInlineQuery(c)
		default:
			return fmt.Errorf("cfd %v cmdType %v not support", c.fd, c.cmdType)
		}
		if err != nil {
			return err
		}
		if !ok { // buffer不满足一个args
			break
		}
		// 处理命令
		if err = processCommand(c); err != nil {
			return err
		}
	}

	if err = server.eventLoop.AddEvent(c.fd, ae.FileEventType_Writeable, addReply, c); err != nil {
		return err
	}
	return nil
}

func processCommand(c *Client) (err error) {
	argStrs := make([]string, 0, 3)
	for _, arg := range c.args {
		argStrs = append(argStrs, arg.ToStr())
	}
	log.Println("argStrs: ", argStrs)

	if cmd := lookupCmd(c); cmd != nil {
		if err := checkLimit(c, cmd); err != nil { // 校验参数个数
			c.reply.Add(NewObjectFromStr(fmt.Sprintf(respFmt, err.Error())))
			return nil
		}
		cmd.fn(c, cmd)
		return nil
	}
	if len(c.args) > 0 { // 找不到命令 对应的回调
		c.reply.Add(NewObjectFromStr(fmt.Sprintf("+<not support %v method>\r\n", c.args[0].ToStr())))
	}
	return nil
}

func addReply(extra interface{}) {
	c, ok := extra.(*Client)
	if !ok || c == nil {
		log.Printf("addReply extra %+v not Client", extra)
		return
	}
	var err error
	for cur := c.reply.Head; cur != nil; {
		node := cur.Val
		if node.gType != GType_Str {
			log.Printf("=== reply type not str")
			continue
		}
		buffer := []byte(node.ToStr())[c.sentLen:]
		c.sentLen, err = Write(c.fd, buffer)
		if err != nil {
			freeClient(c)
			return
		}
		if c.sentLen == len(buffer) {
			next := cur.Next
			cur.Val.decrRefCount()
			c.reply.Del(cur.Val)
			c.sentLen = 0
			cur = next
		}
		break
	}
	if c.reply.Len() <= 0 {
		if err = server.eventLoop.DelEvent(c.fd, ae.FileEventType_Writeable); err != nil {
			log.Printf("del fd %v fileEvent writeAble err: %v", c.fd, err)
			freeClient(c)
		}
		c.sentLen = 0
		return
	}
	// 1. client追加到 pendingClients 里
	// 2. async 在事件循环里，取出pendingClient然后把buffer写入到client端
}

func freeClient(c *Client) {
	// delete read & write file event
	_ = server.eventLoop.DelEvent(c.fd, ae.FileEventType_Readable)
	_ = server.eventLoop.DelEvent(c.fd, ae.FileEventType_Writeable)
	// decrRef reply & args list
	freeClientArgs(c, -1)
	freeClientReply(c)
	// delete from clients
	delete(server.clients, c.fd)
}

func freeClientArgs(c *Client, num int) {
	if num < 0 {
		num = len(c.args)
	}
	for i := 0; i < num; i++ {
		c.args[i].decrRefCount()
	}
	c.args = c.args[num:]
	c.cmdType = cmdType_Unknown
	return
}

func freeClientReply(c *Client) {
	for head := c.reply.Head; head != nil; {
		head.Val.decrRefCount()
		next := head.Next
		c.reply.Del(head.Val)
		head = next
	}

	c.reply = nil
}
