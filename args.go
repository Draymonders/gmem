package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strconv"
)

/*
   参数解析使用
*/

func parseCmdType(c *Client) cmdType {
	if c.cmdType != cmdType_Unknown {
		return c.cmdType
	}
	if c.queryLen <= 0 {
		return cmdType_Unknown
	}
	if c.queryBuf[0] == '*' {
		return cmdType_Bulk
	}
	return cmdType_Inline
}

func handleInlineQuery(c *Client) (ok bool, err error) {
	// todo
	return
}

// Deprecated: handleBulkQuery
// 简单算法实现
func handleBulkQuery(c *Client) (ok bool, err error) {
	// *2\r\n$5\r\nhello\r\n$5\r\nworld\r\n
	var (
		pos  = 0 // 临时指针
		num  = 0 // 当前共有多少个字符串
		args = make([]*Obj, 0, 5)
	)
	// log.Printf("*** client fd %v queryBuf %v \nqueryLen \n%v", c.fd, string(c.queryBuf[:c.queryLen]), c.queryLen)
	numIdx := bytes.Index(c.queryBuf[:c.queryLen], lineSepBytes)
	if numIdx == -1 {
		return false, nil
	}
	pos += numIdx + 2
	num, err = strconv.Atoi(string(c.queryBuf[1:numIdx]))
	if err != nil {
		return false, err
	}
	// log.Printf("=== client fd %v argv %v queryBuf \n%v", c.fd, num, string(c.queryBuf[pos:c.queryLen]))
	for num > 0 {
		if pos >= c.queryLen { // buffer没有读全
			return false, nil
		}
		if c.queryBuf[pos] == '$' { // 复杂字符串
			var strLen int
			if numIdx = bytes.Index(c.queryBuf[pos:c.queryLen], lineSepBytes); numIdx == -1 {
				return false, nil
			}
			if strLen, err = strconv.Atoi(string(c.queryBuf[pos+1 : pos+numIdx])); err != nil {
				return false, err
			}
			pos += numIdx + 2
			if numIdx = bytes.Index(c.queryBuf[pos:c.queryLen], lineSepBytes); numIdx == -1 {
				return false, nil
			}
			if numIdx != strLen {
				// return false, fmt.Errorf("numIdx %v - pos %v != strLen %v", numIdx, pos, strLen)
				log.Printf("err === numIdx %v strLen %v", numIdx, strLen)
				return false, fmt.Errorf("numIdx %v - pos %v != strLen %v", numIdx, pos, strLen)
			}
			args = append(args, NewObjectFromStr(string(c.queryBuf[pos:pos+numIdx])))
			pos += numIdx + 2
		} else {
			panic("not support")
		}
		num--
	}
	c.queryLen -= pos
	c.queryBuf = c.queryBuf[pos:]
	c.args = append(c.args, args...)
	log.Printf("handleBulkQuery client fd %v args %v", c.fd, c.args)
	return true, nil
}

// 流式处理 *2\r\n$5\r\nhello\r\n$5\r\nworld\r\n
func handleBulkQueryStream(c *Client) (ok bool, err error) {
	if c.queryLen <= 0 {
		return false, nil
	}
	if c.bulkNum == 0 { // 目前没有buffer的情况
		// 处理 *2\r\n
		idx, err := c.findLineIndex()
		if err != nil {
			return false, err
		}
		if c.queryBuf[0] != '*' {
			return false, errors.New("expect *")
		}
		c.bulkNum, err = c.extractNum(1, idx)
		if err != nil {
			return false, err
		}
		if c.bulkNum <= 0 {
			return false, nil
		}
	}
	// fmt.Println("bulkNum", c.bulkNum)
	for c.bulkNum > 0 { // 一个个处理
		if c.bulkLen == 0 { // $5\r\n
			numIdx, err := c.findLineIndex()
			if err != nil {
				return false, err
			}
			if c.queryBuf[0] != '$' {
				return false, errors.New("expect $")
			}
			c.bulkLen, err = c.extractNum(1, numIdx)
			if err != nil {
				return false, err
			}
			if c.bulkLen <= 0 {
				return false, nil
			}
		}
		// fmt.Println("bulkLen", c.bulkLen)
		// hello\r\n
		strIdx, err := c.findLineIndex()
		if err != nil {
			return false, err
		}
		if strIdx != c.bulkLen {
			fmt.Println("strIdx ", strIdx, " bulkLen", c.bulkLen)
			return false, errors.New("strIdx != c.bulkLen")
		}
		argStr, err := c.extractStr(0, strIdx) // hello\r\n
		if err != nil {
			return false, err
		}
		c.args = append(c.args, NewObjectFromStr(argStr))
		c.bulkLen = 0
		c.bulkNum--
	}
	return true, nil
}

const lineSepStr = "\r\n" // 分隔符
var lineSepBytes = []byte(lineSepStr)

// 找到换行符
func (c *Client) findLineIndex() (int, error) {
	idx := bytes.Index(c.queryBuf[:c.queryLen], lineSepBytes)
	if idx < 0 || idx >= c.queryLen {
		return idx, errors.New("not found line delimiter")
	}
	return idx, nil
}

// 根据 [st, ed) 获取对应的bytes，转换为数字
func (c *Client) extractNum(st, ed int) (int, error) {
	if st > ed {
		return -1, errors.New("st >= ed")
	}
	v, err := strconv.Atoi(string(c.queryBuf[st:ed]))
	if err != nil {
		return -1, err
	}
	c.queryBuf = c.queryBuf[ed+2:]
	c.queryLen -= ed + 2 // *2\r\n
	return v, nil
}

// 根据[st, ed) 获取对应的bytes，转换为数字
func (c *Client) extractStr(st, ed int) (string, error) {
	if st > ed {
		return "", errors.New("st >= ed")
	}
	if ed > c.queryLen {
		return "", errors.New("query buffer is not enough")
	}
	v := string(c.queryBuf[st:ed])
	c.queryBuf = c.queryBuf[ed+2:]
	c.queryLen -= ed + 2
	return v, nil
}
