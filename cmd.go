package main

import (
	"fmt"
	"strings"
)

var (
	respOK        = "+OK\r\n"
	respFail      = "-%v\r\n"
	respFmt       = "+%s\r\n"
	errArgsNumFmt = "ERR wrong number of arguments for '%s' command"
)

var cmdTable = []*Cmd{
	{name: "COMMAND", limit: 1, fn: Command},
	{name: "SET", limit: 3, fn: Set},
	{name: "GET", limit: 2, fn: Get},
}

type processCmdFn func(*Client, *Cmd)

type Cmd struct {
	name  string
	limit int // 命令支持的个数
	fn    processCmdFn
}

func lookupCmd(c *Client) *Cmd {
	if len(c.args) <= 1 {
		return nil
	}
	v := c.args[0].ToStr()
	for _, cmd := range cmdTable {
		if cmd.name == strings.ToUpper(v) {
			return cmd
		}
	}
	return nil
}

// 校验输入参数格式
func checkLimit(c *Client, cmd *Cmd) error {
	if len(c.args) < cmd.limit {
		return fmt.Errorf(errArgsNumFmt, cmd.name)
	}
	return nil
}

func Command(c *Client, cmd *Cmd) {
	if c == nil {
		return
	}
	c.reply.Add(NewObjectFromStr(respOK))
	freeClientArgs(c, 1)
	return
}

func Set(c *Client, cmd *Cmd) {
	if c == nil {
		return
	}
	k, v := c.args[1], c.args[2]
	var err error
	if obj := c.db.dict.Get(k); obj != nil {
		err = c.db.dict.Set(k, v)
	} else {
		err = c.db.dict.Add(k, v)
	}
	if err != nil {
		c.reply.Add(NewObjectFromStr(fmt.Sprintf(respFail, err.Error())))
	} else {
		c.reply.Add(NewObjectFromStr(respOK))
	}
	freeClientArgs(c, 3)
	return
}

func Get(c *Client, cmd *Cmd) {
	if c == nil {
		return
	}
	k := c.args[1]
	v := c.db.dict.Get(k)

	if v == nil {
		c.reply.Add(NewObjectFromStr(fmt.Sprintf(respFmt, "null")))
	} else {
		c.reply.Add(NewObjectFromStr(fmt.Sprintf(respFmt, v.ToStr())))
	}

	freeClientArgs(c, 2)
	return
}
