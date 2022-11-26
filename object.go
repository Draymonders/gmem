package main

import "strconv"

type GVal interface{}

type GType int

const (
	GType_Str  = 1
	GType_List = 2
	GType_Dict = 3
	Gtype_Set  = 4
	GType_ZSet = 5
)

type Obj struct {
	gType    GType
	ptr      GVal
	refCount int // 引用计数法
}

func NewObjectFromStr(str string) *Obj {
	return &Obj{
		gType:    GType_Str,
		ptr:      str,
		refCount: 1,
	}
}

func NewObject(gType GType, ptr interface{}) *Obj {
	return &Obj{
		gType:    gType,
		ptr:      ptr,
		refCount: 1,
	}
}

func (obj *Obj) incrRefCount() {
	obj.refCount++
}

func (obj *Obj) decrRefCount() {
	obj.refCount--
	if obj.refCount == 0 {
		obj.ptr = nil // go gc
	}
}

func (obj *Obj) ToStr() string {
	if obj == nil {
		return "null"
	}
	if obj.gType == GType_Str {
		return obj.ptr.(string)
	}
	return "<not support>"
}

func (obj *Obj) ToInt64() (int64, error) {
	if obj == nil {
		return -1, nil
	}
	if obj.gType == GType_Str {
		return strconv.ParseInt(obj.ptr.(string), 10, 64)
	}
	return -1, nil
}
