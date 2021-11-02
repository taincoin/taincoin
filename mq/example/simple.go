package main

import (
	"fmt"
	"strconv"

	redismq "github.com/taincoin/taincoin/mq"
)

func main() {
	testQueue := redismq.CreateQueue("localhost", "6379", "", 9, "clicks")
	for i := 0; i < 10; i++ {
		t := strconv.Itoa(i)

		value := "testpayload" + t
		fmt.Println(value)
		fmt.Println(t)

		testQueue.Put(value)
	}
	consumer, err := testQueue.AddConsumer("testconsumer")
	if err != nil {
		panic(err)
	}
	for i := 0; i < 10; i++ {
		p, err := consumer.Get()
		if err != nil {
			fmt.Println(err)
			continue
		}
		fmt.Println(p.CreatedAt)
		fmt.Println(p.Payload)

		err = p.Ack()
		if err != nil {
			fmt.Println(err)
		}
	}
}
