package etcd_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var incKey int64 = 100000

const ttlTime = 10 //ttl的单位都是秒 =

func TestEtcdIfCreate(t *testing.T) {
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		t.Error()
	}
	if etcdClient == nil {
		panic("nil etcd client")
	}
	for i := 1; i <= 1000; i++ {
		key := "/test/batch/" + strconv.Itoa(i)
		tx := etcdClient.Txn(context.Background())
		txnRes, err := tx.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
			Then(clientv3.OpPut(key, fmt.Sprintf("%v", i))).
			Else().
			Commit()
		if err != nil || !txnRes.Succeeded {
			t.Error()
		}
	}
	_ = etcdClient.Close()
}

func BenchmarkEtcdIfCreate(b *testing.B) {
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

	const maxConcurrency = 900
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{}

			go func() {
				defer func() { <-sem }()

				v := int(atomic.AddInt64(&incKey, 1))
				key := "/test/key/" + strconv.Itoa(v)
				tx := etcdClient.Txn(context.Background())
				txnRes, err := tx.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
					Then(clientv3.OpPut(key, fmt.Sprintf("%v", v), clientv3.WithLease(leaseResp.ID))).
					Else().
					Commit()
				if err != nil || !txnRes.Succeeded {
					b.Error(err)
				}
			}()
		}
	})
	wg.Wait()
	//etcdClient.Close()
}
func BenchmarkEtcdIfQuery(b *testing.B) {
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		b.Error()
	}
	if err != nil {
		b.Error()
	}
	b.ResetTimer()

	const maxConcurrency = 900
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{}

			go func() {
				defer func() { <-sem }()

				var ops []clientv3.Op

				for i := 1; i <= 100; i++ {
					ops = append(ops, clientv3.OpGet("/test/batch/"+strconv.Itoa(i)))
				}
				tx := etcdClient.Txn(context.Background())
				txnRes, err := tx.If().Then(ops...).Commit()
				if err != nil || !txnRes.Succeeded {
					b.Error(err)
				}
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
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: []string{"127.0.0.1:2379"}})
	if err != nil {
		b.Error()
	}
	if err != nil {
		b.Error()
	}
	b.ResetTimer()

	const maxConcurrency = 900
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrency)

	b.RunParallel(func(pb *testing.PB) {
		wg.Add(1)
		defer wg.Done()

		for pb.Next() {
			sem <- struct{}{}

			go func() {
				defer func() { <-sem }()
				for i := 1; i <= 100; i++ {
					rsp, err := etcdClient.Get(context.Background(), "/test/batch/"+strconv.Itoa(i))
					if err != nil {
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
