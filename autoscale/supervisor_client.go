package autoscale

import (
	"context"
	"fmt"
	"log"
	"time"

	supervisor "github.com/tikv/pd/supervisor_proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	SupervisorPort  string = "7000"
	IsSupClientMock bool   = true
)

func AssignTenant(podIP string, tenantName string) (resp *supervisor.Result, err error) {
	defer func() {
		if err != nil || (resp != nil && resp.HasErr) {
			log.Printf("[error]failed to AssignTenant, grpc_err: %v  api_err: %v\n", err.Error(), resp.ErrInfo)
		}
	}()
	if !IsSupClientMock {

		conn, err := grpc.Dial(podIP+":"+SupervisorPort, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return nil, err
		}
		if conn != nil {
			defer conn.Close()
		}
		c := supervisor.NewAssignClient(conn)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()
		r, err := c.AssignTenant(ctx, &supervisor.AssignRequest{TenantID: tenantName})
		// if err != nil {
		// 	log.Printf("[error]AssignTenant fail: %v", err)
		// 	return r, err
		// }
		// log.Printf("result: %s", r.HasErr)
		return r, err
	} else {
		time.Sleep(500 * time.Millisecond)
		return &supervisor.Result{HasErr: false}, nil
	}
}

func UnassignTenant(podIP string, tenantName string) (resp *supervisor.Result, err error) {
	defer func() {
		if err != nil || (resp != nil && resp.HasErr) {
			log.Printf("[error]failed to UnassignTenant, grpc_err: %v  api_err: %v\n", err.Error(), resp.ErrInfo)
		}
	}()
	if !IsSupClientMock {
		conn, err := grpc.Dial(podIP+":"+SupervisorPort, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return nil, err
		}
		if conn != nil {
			defer conn.Close()
		}
		c := supervisor.NewAssignClient(conn)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()
		r, err := c.UnassignTenant(ctx, &supervisor.UnassignRequest{AssertTenantID: tenantName})
		// if err != nil {
		// 	log.Printf("[error]AssignTenant fail: %v", err)
		// 	return r, err
		// }
		// log.Printf("result: %s", r.HasErr)
		return r, err
	} else {
		time.Sleep(500 * time.Millisecond)
		return &supervisor.Result{HasErr: false}, nil
	}
}

func GetCurrentTenant(podIP string) (resp *supervisor.GetTenantResponse, err error) {
	defer func() {
		if err != nil {
			log.Printf("[error]failed to GetCurrentTenant, grpc_err: %v\n", err.Error())
		}
	}()
	if !IsSupClientMock {
		conn, err := grpc.Dial(podIP+":"+SupervisorPort, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			return nil, err
		}
		if conn != nil {
			defer conn.Close()
		}
		c := supervisor.NewAssignClient(conn)

		// Contact the server and print out its response.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r, err := c.GetCurrentTenant(ctx, &emptypb.Empty{})
		// if err != nil {
		// 	log.Printf("[error]AssignTenant fail: %v", err)
		// 	return r, err
		// }
		// log.Printf("result: %s", r.HasErr)
		return r, err
	} else {
		time.Sleep(500 * time.Millisecond)
		return nil, fmt.Errorf("mock GetCurrentTenant")
	}
}
