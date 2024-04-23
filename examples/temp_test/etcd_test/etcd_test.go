package etcd_test

import (
	"context"
	"fmt"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

var incKey int64 = 100000

const ttlTime = 100 //ttl的单位都是秒 =

func TestEtcdIfCreate(t *testing.T) {
	// 假设你已经有了一个etcd客户端cli
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		t.Error()
	}

	for i := 1; i <= 1000; i++ {
		// 在这里执行你的etcd操作
		key := "/test/batch/" + strconv.Itoa(i)
		tx := etcdClient.Txn(context.Background())
		txnRes, err := tx.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
			Then(clientv3.OpPut(key, fmt.Sprintf("%v", i))).
			Else().
			Commit()
		if err != nil || !txnRes.Succeeded { //抢锁失败
			t.Error()
		}
	}
	etcdClient.Close()
}

func BenchmarkEtcdIfCreate(b *testing.B) {
	// 假设你已经有了一个etcd客户端cli
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		b.Error()
	}
	lease := clientv3.NewLease(etcdClient)
	leaseResp, err := lease.Grant(context.Background(), ttlTime)
	if err != nil {
		b.Error()
	}
	b.ResetTimer()

	// 限制并发数
	const maxConcurrency = 900
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{} // 获取一个信号量

			go func() {
				defer func() { <-sem }() // 释放信号量

				// 在这里执行你的etcd操作
				v := int(atomic.AddInt64(&incKey, 1))
				key := "/test/key/" + strconv.Itoa(v)
				tx := etcdClient.Txn(context.Background())
				txnRes, err := tx.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
					Then(clientv3.OpPut(key, fmt.Sprintf("%v", v), clientv3.WithLease(leaseResp.ID))).
					Else().
					Commit()
				if err != nil || !txnRes.Succeeded { //抢锁失败
					b.Error(err)
				}
			}()
		}
	})
	wg.Wait()
	//etcdClient.Close()
}
func BenchmarkEtcdIfQuery(b *testing.B) {
	// 假设你已经有了一个etcd客户端cli
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		b.Error()
	}
	if err != nil {
		b.Error()
	}
	b.ResetTimer()

	// 限制并发数
	const maxConcurrency = 900
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{} // 获取一个信号量

			go func() {
				defer func() { <-sem }() // 释放信号量

				ops := []clientv3.Op{}

				// 构造多个查询key是否存在的操作
				for i := 1; i <= 100; i++ {
					ops = append(ops, clientv3.OpGet("/test/batch/"+strconv.Itoa(i)))
				}
				tx := etcdClient.Txn(context.Background())
				txnRes, err := tx.If().Then(ops...).Commit()
				if err != nil || !txnRes.Succeeded { //抢锁失败
					b.Error(err)
				}
				// 遍历结果
				for _, r := range txnRes.Responses {
					rv, ok := r.Response.(*etcdserverpb.ResponseOp_ResponseRange)
					if !ok {
						b.Error()
					} else {
						if rv.ResponseRange.Count != 1 {
							b.Error()
						}
					}
				}
			}()
		}
	})
	wg.Wait()
	//etcdClient.Close()
}

func BenchmarkEtcdNormalBatchQuery(b *testing.B) {
	// 假设你已经有了一个etcd客户端cli
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		b.Error()
	}
	if err != nil {
		b.Error()
	}
	b.ResetTimer()

	// 限制并发数
	const maxConcurrency = 900
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{} // 获取一个信号量

			go func() {
				defer func() { <-sem }() // 释放信号量
				for i := 1; i <= 100; i++ {
					rsp, err := etcdClient.Get(context.Background(), "/test/batch/"+strconv.Itoa(i))
					if err != nil { //抢锁失败
						b.Error(err)
					}
					if rsp.Count != 1 {
						b.Error()
					}
				}
			}()
		}
	})
	wg.Wait()
	//etcdClient.Close()
}
