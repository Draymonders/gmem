package main

type Node struct {
	Val  *Obj
	Pre  *Node
	Next *Node
}

type ListType struct {
	EqualFn func(*Node, *Node) bool
}

func ListEqualFn(a *Node, b *Node) bool {
	if a == nil && b == nil {
		return true
	} else if a == nil && b != nil {
		return false
	} else if a != nil && b == nil {
		return false
	}
	// a, b not nil
	if a.Val.gType != b.Val.gType {
		return false
	}
	return a.Val.ptr == b.Val.ptr
}

type List struct {
	ListType
	Head *Node
	Tail *Node
	len  int
}

func NewList(listType ListType) *List {
	return &List{ListType: listType}
}

func (l *List) Len() int {
	return l.len
}

func (l *List) Add(val *Obj) {
	node := &Node{Val: val}
	if l.Head == nil {
		l.Head = node
		l.Tail = node
	} else {
		node.Pre = l.Tail
		l.Tail.Next = node

		l.Tail = node
	}
	l.len++
}

func (l *List) FindByVal(val *Obj) *Node {
	node := &Node{Val: val}
	for cur := l.Head; cur != nil; cur = cur.Next {
		if l.EqualFn(node, cur) {
			return cur
		}
	}
	return nil
}

func (l *List) Del(val *Obj) {
	if cur := l.FindByVal(val); cur != nil {
		pre, next := cur.Pre, cur.Next
		if pre == nil {
			l.Head = next
		} else {
			pre.Next = next
		}
		if next == nil {
			l.Tail = pre
		} else {
			next.Pre = pre
		}
		cur.Next = nil
		cur.Pre = nil
		cur.Val.decrRefCount()
		l.len--
	}
}
