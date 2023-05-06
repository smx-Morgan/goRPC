## goRPC 框架实现

### 服务端的编码解码

### 高性能客户端

### 服务注册功能
我们可以将结构体的方法映射为服务。使其像调用本地程序一样调用远程服务。

这里我们通过反射实现service

### Http支持：
Server实现handler接口，只接受HTTP CONNECT的请求，并Hijack这个http的tcp连接来做Client和Server之间通信的conn，而之前是直接用Server Accept一个TCP listener，然后做通信。


debugHTTP持Server变量，实现handler接口，处理函数里用持有的Server变量做一些debug相关的统计，这时候可以通过HTTP请求获取到对应的Server的一些调用状态。

HTTP 协议转化为 RPC 协议的过程是包装了的，使用者不感知，客户端的协议转换过程已经在 NewHTTPClient 里实现了。