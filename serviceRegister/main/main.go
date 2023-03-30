package main

import (
	"context"
	"goRPC"
	"log"
	"net"
	"sync"
	"time"
)

type Foo int

type Args struct{ Num1, Num2 int }

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

// 注册 Foo 到 Server 中，并启动 RPC 服务
func startServer(addr chan string) {
	var foo Foo
	if err := goRPC.Register(&foo); err != nil {
		log.Fatal("register error:", err)
	}
	// pick a free port
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	goRPC.Accept(l)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)
	client, _ := goRPC.Dial("tcp", <-addr)
	defer func() { _ = client.Close() }()
	time.Sleep(time.Second)
	args := &Args{Num1: 1, Num2: 50}
	var reply int
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	if err := client.Call(ctx, "Foo.Sum", args, &reply); err != nil {
		log.Fatal("call Foo.Sum error:", err)
		log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
	}
	log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
	//发送请求和接收响应
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i * i}
			var reply int
			ctx, _ := context.WithTimeout(context.Background(), time.Second)
			if err := client.Call(ctx, "Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
}
