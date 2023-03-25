package serviceRegister

import (
	"go/ast"
	"log"
	"reflect"
	"sync/atomic"
)

//反射的类型对象（reflect.Type）
//反射中，当需要区分一个大品种的类型时，就会用到种类（Kind）。例如需要统一判断类型中的指针时，使用种类（Kind）信息就较为方便。

// 通过反射动态获取结构体方法
type MethodType struct {
	method    reflect.Method //方法本身
	ArgType   reflect.Type   //第一个参数类型
	ReplyType reflect.Type   //第二个参数类型
	numCalls  uint64         //统计方法的调用次数
}

func (m *MethodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}
func (m *MethodType) NewArgv() reflect.Value {
	var argv reflect.Value
	//可以通过 reflect.Elem() 方法获取这个指针指向的元素类型，这个获取过程被称为取元素
	if m.ArgType.Kind() == reflect.Ptr {
		argv = reflect.New(m.ArgType.Elem())
	} else {
		argv = reflect.New(m.ArgType).Elem()
	}
	return argv
}
func (m *MethodType) NewReplyv() reflect.Value {
	// reply must be a pointer type
	replyv := reflect.New(m.ReplyType.Elem())
	switch m.ReplyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.ReplyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.ReplyType.Elem(), 0, 0))
	}
	return replyv
}

type Service struct {
	Name   string
	typ    reflect.Type
	rcvr   reflect.Value
	Method map[string]*MethodType
}

// 构建服务的结构实例
func NewService(rcvr interface{}) *Service {
	s := new(Service)
	s.rcvr = reflect.ValueOf(rcvr)
	s.Name = reflect.Indirect(s.rcvr).Type().Name()
	s.typ = reflect.TypeOf(rcvr)
	if !ast.IsExported(s.Name) {
		log.Fatalf("rpc server: %s is not a valid service name", s.Name)
	}
	s.registerMethods()
	return s
}
func (s *Service) registerMethods() {
	s.Method = make(map[string]*MethodType)
	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type
		//两个导出或内置类型的入参（反射时为 3 个，第 0 个是自身，类似于 python 的 self，java 中的 this
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}
		//返回值有且只有 1 个，类型为 error
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		s.Method[method.Name] = &MethodType{
			method:    method,
			ArgType:   argType,
			ReplyType: replyType,
		}
		log.Printf("rpc server: register %s.%s\n", s.Name, method.Name)

	}
}
func isExportedOrBuiltinType(t reflect.Type) bool {
	return ast.IsExported(t.Name()) || t.PkgPath() == ""
}

// 通过反射值调用方法。
func (s *Service) Call(m *MethodType, argv, replyv reflect.Value) error {
	//numCall +1
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func
	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}
