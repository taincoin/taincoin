# Installing Redis on Ubuntu

```
$ sudo apt-get install redis-server
```

After installation is complete, check the version.
```
$ redis-server --version
```

Now we will open the redis.conf file to determine the maximum amount of memory Redis can use and how to delete data when the maximum amount of memory is exceeded.

```
sudo vi /etc/redis/redis.conf
```
Find maxmemory and maxmemory-policy in the configuration file and change it as follows. The maximum amount of memory used is set at 1G, and when the maximum memory is exceeded, the oldest data is erased to secure memory, and the most recently stored data is set to be used.

```
maxmemory 1g
maxmemory-policy allkeys-lru
```

Restart Redis for the settings to take effect

'''
$ sudo systemctl restart redis-server.service
'''

The default port for Redis is 6379. Check if Redis is using port 6379

'''
$ netstat -nlpt | grep 6379
'''

Connect to the Redis server with the following command.

'''
$ redis-cli
'''

Enter data by key-value using set

'''
set [key] [value]
'''

Retrieve the data value with get [key].
'''
get [key]
'''

You can also retrieve the stored key.
'''
keys *
'''


Note: This project is no longer actively maintained. Please refer to its spiritual successor [rmq].

[rmq]: https://github.com/adjust/rmq

--


### Basics

To get started you need a running redis server. Since the tests run `FlushDB()` an otherwise unused database is highly recommended
The first step is to create a new queue:
```go
package main

import (
	"fmt"
	"github.com/taincoin/taincoin/mq"
)

func main() {
	testQueue := redismq.CreateQueue("localhost", "6379", "", 9, "clicks")
	...
}
```
To write into the queue you simply use `Put()`:
```go
	...
	testQueue := redismq.CreateQueue("localhost", "6379", "", 9, "clicks")
	testQueue.Put("testpayload")
	...
}
```
The payload can be any kind of string, yes even a [10MB one]

To get messages out of the queue you need a consumer:
```go
	...
	consumer, err := testQueue.AddConsumer("testconsumer")
	if err != nil {
		panic(err)
	}
	package, err := consumer.Get()
	if err != nil {
		panic(err)
	}
	fmt.Println(package.Payload)
	...
}
```
`Payload` will hold the original string, while `package` will have some additional header information.

To remove a package from the queue you have to `Ack()` it:
```go
	...
	package, err := consumer.Get()
	if err != nil {
		panic(err)
	}
	err = package.Ack()
	if err != nil {
		panic(err)
	}
	...
}
```

### Buffered Queues

When input speed is of the essence `BufferedQueues` will scratch that itch.
They pipeline multiple puts into one fast operation. The only issue is that upon crashing or restart the packages in
the buffer that haven't been written yet will be lost. So it's advised to wait one second before terminating your program to flush the buffer.

The usage is as easy as it gets:
```go
	...
	bufferSize := 100
	testQueue := redismq.CreateBufferedQueue("localhost", "6379", "", 9, "clicks", bufferSize)
	testQueue.Start()
	...
}
```
`Put()` and `Get()` stay exactly the same.
I have found anything over 200 as `bufferSize` not to increase performance any further.

To ensure that no packages are left in the buffer when you shut down your program you need to call
`FlushBuffer()` which will tell the queue to flush the buffer and wait till it's empty.
```go
	testQueue.FlushBuffer()
```

### Multi Get

Like `BufferedQueues` for `Get()` `MultiGet()` speeds up the fetching of messages. The good news it comes without the buffer loss issues.

Usage is pretty straight forward with the only difference being the `MultiAck()`:
```go
	...
	packages, err := consumer.MultiGet(100)
	if err != nil {
		panic(err)
	}
	for i := range packages {
		fmt.Println(p[i].Payload)
	}
	packages[len(p)-1].MultiAck()
	...
}
```
`MultiAck()` can be called on any package in the array with all the prior packages being "acked". This way you can `Fail()` single packages.

### Reject and Failed Queues

Similar to AMQP redismq supports `Failed Queues` meaning that packages that are rejected by a consumer will be stored in separate queue for further inspection. Alternatively a consumer can also `Requeue()` a package and put it back into the queue:
```go
	...
	package, err := consumer.Get()
	if err != nil {
		panic(err)
	}
	err = package.Requeue()
	if err != nil {
		panic(err)
	}
	...
}
```

To push the message into the `Failed Queue` of this consumer simply use `Fail()`:
```go
	...
	package, err := consumer.Get()
	if err != nil {
		panic(err)
	}
	err = package.Fail()
	if err != nil {
		panic(err)
	}
	package, err = suite.consumer.GetUnacked()
	...
}
```
As you can see there is also a command to get messages from the `Failed Queue`.

## How fast is it

Even though the original implementation wasn't aiming for high speeds the addition of `BufferedQueues` and `MultiGet`
make it go something like [this](http://www.youtube.com/watch?feature=player_detailpage&v=sGBMSLvggqA#t=58).

All of the following benchmarks were conducted on a MacBook Retina with a 2.4 GHz i7.
The InputRate is the number of messages per second that get inserted, WorkRate the messages per second consumed.

Single Publisher, Two Consumers only atomic `Get` and `Put`
```
InputRate:	12183
WorkRate:	12397
```

Single Publisher, Two Consumers using `BufferedQueues` and `MultiGet`
```
InputRate:	46994
WorkRate:	25000
```

And yes that is a persistent message queue that can move over 70k messages per second.

If you want to find out for yourself checkout the `example` folder. The `load.go` or `buffered_queue.go`
will start a web server that will display performance stats under `http://localhost:9999/stats`.