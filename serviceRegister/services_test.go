package serviceRegister

import (
	"fmt"
	"reflect"
	"testing"
)

type Foo int

type Args struct{ Num1, Num2 int }

// 导出方法
func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

// 非导出方法（私有方法）
func (f Foo) sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed: "+msg, v...))
	}
}

// 测试newService方法
func TestNewService(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	_assert(len(s.Method) == 1, "wrong service Method, expect 1, but got %d", len(s.Method))
	mType := s.Method["Sum"]
	_assert(mType != nil, "wrong Method, Sum shouldn't nil")

}

// 测试call函数
func TestMethodType_Calls(t *testing.T) {
	var foo Foo
	s := NewService(&foo)
	mType := s.Method["Sum"]

	argv := mType.NewArgv()
	replyv := mType.NewReplyv()
	argv.Set(reflect.ValueOf(Args{Num1: 1, Num2: 3}))
	err := s.Call(mType, argv, replyv)
	_assert(err == nil && *replyv.Interface().(*int) == 4 && mType.NumCalls() == 1, "failed to call Foo.Sum")
}