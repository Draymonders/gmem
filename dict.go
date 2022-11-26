package main

import (
	"errors"
	"hash/crc32"
	"log"
	"math/rand"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}

var (
	errorExist    = errors.New("key exist")
	errorNotFound = errors.New("key not found")
)

const (
	loadFactor = 1  // 负载因子
	initSize   = 16 // 初始hTable bucket数
)

type DictType struct {
	HashFn  func(key *Obj) int
	EqualFn func(k1, k2 *Obj) bool
}

type Dict struct {
	DictType
	ht        [2]*hTable // 惰性加载
	rehashIdx int        // -1 没在扩容，其余表示在扩容
}

type hTable struct {
	entries []*hEntry
	size    int // size must be power(2, x)
	mask    int // size - 1
	used    int
}

type hEntry struct {
	key  *Obj
	val  *Obj
	next *hEntry
}

func Hash(key *Obj) int {
	if key.gType != GType_Str {
		log.Printf("key %+v type not str", key)
		return -1
	}
	return int(crc32.ChecksumIEEE([]byte(key.ptr.(string))))
}

func Equal(k1, k2 *Obj) bool {
	if k1.gType != GType_Str || k2.gType != GType_Str {
		return false
	}
	return k1.ptr.(string) == k2.ptr.(string)
}

func NewDict(dictType DictType) *Dict {
	return &Dict{
		DictType:  dictType,
		rehashIdx: -1,
	}
}

func newHTable(size int) *hTable {
	return &hTable{
		entries: make([]*hEntry, size),
		size:    size,
		mask:    size - 1,
		used:    0,
	}
}

func (d *Dict) RandomGet() *hEntry {
	bucketNum := 0
	if d.ht[0] == nil {
		return nil
	}
	if d.isRehash() && d.ht[1].used > d.ht[0].used {
		bucketNum = 1
	}
	bucket := d.ht[bucketNum]
	if bucket.used == 0 {
		return nil
	}
	th := rand.Intn(bucket.used)

	// 寻找 bucket里面 第th个entry
	id := 0
	for i := 0; i < len(bucket.entries); i++ {
		if bucket.entries[i] == nil {
			continue
		}
		for cur := bucket.entries[i]; cur != nil; cur = cur.next {
			if id == th {
				return cur
			}
			id++
		}
	}
	return nil
}

func (d *Dict) Get(key *Obj) *Obj {
	d.expandIfNeed()
	if d.isRehash() {
		if v := d.get(key, 1); v != nil {
			return v
		}
	}
	return d.get(key, 0)
}

func (d *Dict) Add(key, val *Obj) error {
	d.expandIfNeed()
	if d.isRehash() {
		if v := d.get(key, 1); v != nil {
			return errorExist
		}
	}
	if v := d.get(key, 0); v != nil {
		return errorExist
	}
	d.add(key, val)
	return nil
}

func (d *Dict) Set(key, val *Obj) error {
	d.expandIfNeed()

	var oldV *Obj
	bucketNum := 0

	if oldV = d.get(key, 0); oldV != nil {
		bucketNum = 0
	} else if d.isRehash() {
		if oldV = d.get(key, 1); oldV != nil {
			bucketNum = 1
		}
	}
	if oldV == nil {
		return errorNotFound
	}

	d.set(key, val, oldV, bucketNum)
	return nil
}

func (d *Dict) Del(key *Obj) bool {
	d.expandIfNeed()

	exist := false
	for i := 0; i <= 1; i++ { // 这种写法不错
		idx := d.HashFn(key) & d.ht[i].mask
		cur := d.ht[i].entries[idx]
		var pre *hEntry
		for cur != nil {
			next := cur.next
			if d.EqualFn(cur.key, key) {
				exist = true
				cur.val.decrRefCount()
				cur.key.decrRefCount()
				if pre == nil {
					d.ht[i].entries[idx] = next
					break
				} else {
					pre.next = next
					break
				}
			}
			pre = cur
			cur = next
		}
		if !d.isRehash() {
			break
		}
	}
	return exist
}

func (d *Dict) set(key, newVal, oldVal *Obj, bucketNum int) {
	oldVal.decrRefCount() // help go gc
	oldVal.ptr = newVal.ptr
	oldVal.incrRefCount()
}

func (d *Dict) get(key *Obj, bucketNum int) *Obj {
	idx := d.HashFn(key) & d.ht[bucketNum].mask
	for cur := d.ht[bucketNum].entries[idx]; cur != nil; cur = cur.next {
		if d.EqualFn(cur.key, key) {
			return cur.val
		}
	}
	return nil
}

func (d *Dict) add(key, val *Obj) {
	bucketNum := 0
	if d.isRehash() {
		bucketNum = 1
	}

	idx := d.HashFn(key) & d.ht[bucketNum].mask
	entry := &hEntry{
		key:  key,
		val:  val,
		next: d.ht[bucketNum].entries[idx],
	}
	key.incrRefCount()
	val.incrRefCount()

	d.ht[bucketNum].entries[idx] = entry
	d.ht[bucketNum].used++
}

// 是否需要扩容
func (d *Dict) needResize() bool {
	return d.ht[0].used/d.ht[0].size >= loadFactor
}

// 是否正在扩容
func (d *Dict) isRehash() bool {
	return d.rehashIdx != -1
}

// 下个要扩容的数量
func (d *Dict) nextExpandSize() int {
	return d.ht[0].size << 1
}

func (d *Dict) expandIfNeed() {
	// 如果需要扩容的话，rehash下
	if d.ht[0] == nil {
		d.ht[0] = newHTable(initSize)
	} else if d.isRehash() {
		d.expandStep()
	} else if d.needResize() {
		sz := d.nextExpandSize()
		d.ht[1] = newHTable(sz)
		d.expandStep()
	}
}

// 扩容 d.ht[0] -> d.ht[1]
func (d *Dict) expandStep() {
	d.rehashIdx++
	if d.rehashIdx >= len(d.ht[0].entries) || d.ht[0].used == 0 {
		d.finishRehash()
		return
	}
	// 每次迁移一个bucket，其实可以考虑按阈值迁移
	// 对于操作都是单操作的来说，不会出现已经在rehash的情况下，又需要rehash的情况
	fromHt := d.ht[0]
	toHt := d.ht[1]

	rehashNum := 0
	for i := d.rehashIdx; ; i++ {
		if i >= len(fromHt.entries) {
			d.finishRehash()
			return
		}
		if fromHt.entries[i] == nil {
			continue
		}
		fromBucket := fromHt.entries[i]
		cur := fromBucket
		for cur != nil {
			next := cur.next

			newBucketId := d.HashFn(cur.key) & toHt.mask
			cur.next = toHt.entries[newBucketId]
			toHt.entries[newBucketId] = cur

			toHt.used++
			fromHt.used--
			cur = next
			rehashNum++
		}
		d.rehashIdx = i
		break
	}
}

// 完成rehash
func (d *Dict) finishRehash() {
	d.ht[0] = d.ht[1]
	d.ht[1] = nil
	d.rehashIdx = -1
}
