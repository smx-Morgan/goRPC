## goRPC 框架实现
一个典型的 RPC : 

```go
err = client.Call("Arith.Multiply", args, &reply)
```

服务名 Arith，方法名 Multiply，参数 args 三个，服务端的响应包括错误 error，返回值 reply 2 个。我们将请求和响应中的参数和返回值抽象为 body，剩余的信息放在 header 中。
所以header中就会有： 服务名，请求的序号，以及错误信息

#### 消息的序列化与反序列化

构建RPC的序列化以及反序列化 我们需要定义编码方式，对消息头head和 消息body 进行编码和解码。在这个简易式rpc中我们仅支持json

定义 `Codec` 结构体，这个结构体由四部分构成，

`conn` 是由构建函数传入，通常是通过 TCP 或者 Unix 建立 socket 时得到的链接实例，

dec 和 enc 对应 gob 的 Decoder 和 Encoder，

buf 是为了防止阻塞而创建的带缓冲的 `Writer`

```go
type Codec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}
```

对于自己的RPC架构的编码方式我们需要编写



实现 `ReadHeader`、`ReadBody`、`Write` 和 `Close`

客户端与服务端的通信需要协商一些内容，例如 HTTP 报文，分为 header 和 body 2 部分，body 的格式和长度通过 header 中的 `Content-Type` 和 `Content-Length` 指定，服务端通过解析 header 就能够知道如何从 body 中读取需要的信息。对于 RPC 协议来说，这部分协商是需要自主设计的。为了提升性能，一般在报文的最开始会规划固定的字节，来协商相关的信息。比如第1个字节用来表示序列化方式，第2个字节表示压缩方式，第3-6字节表示 header 的长度，7-10 字节表示 body 的长度



实现了 `Accept` 方式，`net.Listener` 作为参数，for 循环等待 socket 连接建立，并开启子协程处理，处理过程交给了 `ServerConn` 方法

Accept 连接：（防止耦合）

```go
lis, _ := net.Listen("tcp", ":9999")
geerpc.Accept(lis)
```



建立连接，首先我们需要先解码采用 JSON 编码请求 Option（请求包含magicNumber 和 编码解码方式）

然后确定 MagicNumber （相当于是服务和客户端key确定是否是本rpc框架发起的请求）

再然后通过 Option 的 CodeType 解码剩余的内容。

解码完成之后我们则可以处理处理请求

`serveCodec` 的过程非常简单。主要包含三个阶段

- 读取请求 readRequest
- 处理请求 handleRequest
- 回复请求 sendResponse

在一次连接中，允许接收多个请求，即多个 request header 和 request body，因此这里使用了 for 无限制地等待请求的到来，直到发生错误

**注意**

handleRequest 使用了协程并发执行请求

处理请求是并发的，但是回复请求的报文必须是逐个发送的，并发容易导致多个回复报文交织在一起，客户端无法解析。在这里使用锁(sending)保证。

尽力而为，只有在 header 解析失败时，才终止循环

客户端：

在 `startServer` 中使用了信道 `addr`，确保服务端端口监听成功，客户端再发起请求。

客户端首先发送 `Option` 进行协议交换，接下来发送消息头 ，和消息体 。

最后解析服务端的响应 `reply`，并打印出来。



#### 高性能客户端

首先定义call结构体来承载一次 RPC 调用所需要的信息。：

```go
// Call represents an active RPC.
type Call struct {
	Seq           uint64
	ServiceMethod string      // format "<service>.<method>"
	Args          interface{} // arguments to the function
	Reply         interface{} // reply from the function
	Error         error       // if error occurs, it will be set
	Done          chan *Call  // Strobes when call is complete.
}
```

直观一点的的函数：

```go
func (t *T) MethodName(argType T1, replyType *T2) error
```

#### 服务注册功能

我们可以将结构体的方法映射为服务。使其像调用本地程序一样调用远程服务。

这里我们通过反射实现service

### Http支持：
Server实现handler接口，只接受HTTP CONNECT的请求，并Hijack这个http的tcp连接来做Client和Server之间通信的conn，而之前是直接用Server Accept一个TCP listener，然后做通信。


debugHTTP持Server变量，实现handler接口，处理函数里用持有的Server变量做一些debug相关的统计，这时候可以通过HTTP请求获取到对应的Server的一些调用状态。

HTTP 协议转化为 RPC 协议的过程是包装了的，使用者不感知，客户端的协议转换过程已经在 NewHTTPClient 里实现了。

### 均衡负载
假设有多个服务实例，每个实例提供相同的功能，为了提高整个系统的吞吐量，每个实例部署在不同的机器上。客户端可以选择任意一个实例进行调用，获取想要的结果。那如何选择呢？取决了负载均衡的策略。对于 RPC 框架，我们可以很容易地想到这么几种策略：

随机选择策略 - 从服务列表中随机选择一个。
轮询算法(Round Robin) - 依次调度不同的服务器，每次调度执行 i = (i + 1) mode n。
加权轮询(Weight Round Robin) - 在轮询算法的基础上，为每个服务实例设置一个权重，高性能的机器赋予更高的权重，也可以根据服务实例的当前的负载情况做动态的调整，例如考虑最近5分钟部署服务器的 CPU、内存消耗情况。
哈希/一致性哈希策略 - 依据请求的某些特征，计算一个 hash 值，根据 hash 值将请求发送到对应的机器。一致性 hash 还可以解决服务实例动态添加情况下，调度抖动的问题。一致性哈希的一个典型应用场景是分布式缓存服务。感兴趣可以阅读动手写分布式缓存 - GeeCache第四天 一致性哈希(hash)

### 注册中心

服务端启动后，向注册中心发送注册消息，注册中心得知该服务已经启动，处于可用状态。一般来说，服务端还需要定期向注册中心发送心跳，证明自己还活着。
客户端向注册中心询问，当前哪天服务是可用的，注册中心将可用的服务列表返回客户端。
客户端根据注册中心得到的服务列表，选择其中一个发起调用。

