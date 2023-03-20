package Client

import (
	"encoding/json"
	"errors"
	"fmt"
	"goRPC/Maincodec"
	"goRPC/Maincodec/codec"
	"io"
	"log"
	"net"
	"sync"
)

type Call struct {
	Seq           uint64
	ServiceMethod string      // 函数标准"<service>.<method>"
	Args          interface{} // 参数
	Reply         interface{} // 返回值
	Error         error       //报错
	Done          chan *Call  // 呼叫完成时选通
}

// 支持异步调用，当调用结束的时候用call.done通知调用方
func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc      codec.Codec       //编码器
	opt     *Maincodec.Option //编码器选择
	sending sync.Mutex        //互斥锁，和服务端类似，为了保证请求的有序发送
	header  codec.Header      //消息头
	mu      sync.Mutex
	seq     uint64           //用于给发送的请求编号，每个请求拥有唯一编号
	pending map[uint64]*Call //存储未处理完的请求，key是seq
	//closing 和 shutdown 任意一个值置为 true，则表示 Client 处于不可用的状态
	closing  bool // 用户主动关闭的
	shutdown bool // shutdown 置为 true 一般是有错误发生
}

// io.Closer需要实现
var _ io.Closer = (*Client)(nil)

// 报错信息
var ErrShutdown = errors.New("connection is shut down")

// 关闭连接
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	//关闭链接实例
	return client.cc.Close()
}

func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}
	//将参数 call 添加到 client.pending 中，并更新 client.seq
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	//根据 seq，从 client.pending 中移除对应的 call，并返回
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

// 服务端或客户端发生错误时调用，将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call。
func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}
func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		//call 不存在，可能是请求没有发送完整，或者因为其他原因被取消
		case call == nil:
			err = client.cc.ReadBody(nil)
		//call 存在，但服务端处理出错，即 h.Error 不为空
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		//call 存在，服务端处理正常，那么需要从 body 中读取 Reply 的值。
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body" + err.Error())
			}
			call.done()
		}
	}
	client.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *Maincodec.Option) (*Client, error) {
	//先完成协议交换
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	// send options with server
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}
func newClientCodec(cc codec.Codec, opt *Maincodec.Option) *Client {
	client := &Client{
		seq:     1, // seq starts with 1, 0 means invalid call
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	//接收
	go client.receive()
	return client
}

// 接收参数
func parseOptions(opts ...*Maincodec.Option) (*Maincodec.Option, error) {
	// if opts is nil or pass nil as parameter
	if len(opts) == 0 || opts[0] == nil {
		return Maincodec.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = Maincodec.DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = Maincodec.DefaultOption.CodecType
	}
	return opt, nil
}

// Dial 函数，便于用户传入服务端地址，创建 Client 实例。
func Dial(network, address string, opts ...*Maincodec.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	// close the connection if client is nil
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn, opt)
}
func (client *Client) send(call *Call) {
	//确保客户端将发送一个完整的请求
	client.sending.Lock()
	defer client.sending.Unlock()

	// 注册请求创建其序列号
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	//创建请求头
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	// 编码
	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		// call为nil表示write部分失效
		// 客户端收到响应并进行处理
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go 和 Call 是客户端暴露给用户的两个 RPC 服务调用接口，Go 是一个异步接口，返回 call 实例。
// go异步调用函数
// 返回Call结构体
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// RPC调用函数名
// 返回错误值
func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}
