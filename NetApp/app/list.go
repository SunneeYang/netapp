package main

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

func producer_list(queue *list.List, mutex *sync.Mutex, c chan bool)  {
	for i := 0; i < 1000000; i++ {
		mutex.Lock()
		queue.PushBack(i)
		mutex.Unlock()
	}

	c <- true
}

func comsumer_list(queue *list.List, mutex *sync.Mutex, cp chan bool, cc chan bool)  {
	for  {


		if queue.Len() <= 0 {
			select {
			case <- cp:
				cc <- true
				return

			default:

			}
		} else {
			mutex.Lock()
			val := queue.Front()
			queue.Remove(val)
			mutex.Unlock()
		}

	}

}

func producer_chan(queue chan int, cp chan bool)  {
	for i := 0; i < 1000000; i++ {
		queue <- i
	}

	cp <- true
}

func comsumer_chan(queue chan int, cp chan bool, cc chan bool)  {
	for {
		if len(queue) <= 0 {
			select {
			case <- cp:
				cc <- true
				return

			default:

			}
		} else {
			<-queue
		}

	}
}

func main() {
	//doubleList := list.New()
	queue := make(chan int, 10000)
	c_c := make(chan bool)
	c_p := make(chan bool)
	//var mutex sync.Mutex

	lastTime := time.Now()

	//go producer_list(doubleList, &mutex, c_p)
	//go comsumer_list(doubleList, &mutex, c_p, c_c)

	go producer_chan(queue, c_p)
	go comsumer_chan(queue, c_p, c_c)

	<-c_c

	fmt.Println(time.Now().Sub(lastTime).Nanoseconds())
}
