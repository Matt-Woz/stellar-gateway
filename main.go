package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/couchbase/gocb/v2"
	admin_bucket_v1 "github.com/couchbase/stellar-nebula/genproto/admin/bucket/v1"
	analytics_v1 "github.com/couchbase/stellar-nebula/genproto/analytics/v1"
	data_v1 "github.com/couchbase/stellar-nebula/genproto/data/v1"
	query_v1 "github.com/couchbase/stellar-nebula/genproto/query/v1"
	routing_v1 "github.com/couchbase/stellar-nebula/genproto/routing/v1"
	search_v1 "github.com/couchbase/stellar-nebula/genproto/search/v1"
	"github.com/couchbase/stellar-nebula/server"
	"google.golang.org/grpc"
)

var remoteHost = flag.String("host", "couchbase://127.0.0.1", "the remote host to link to")
var remoteUser = flag.String("user", "Administrator", "the username to use for the remote host")
var remotePass = flag.String("pass", "password", "the password to use for the remote host")
var localHostname = flag.String("local-hostname", "127.0.0.1", "the local hostname for this service")
var port = flag.Uint64("port", 18098, "the port to listen on")

func main() {
	flag.Parse()

	log.Printf("linking to remote host at: %s (user: %s)", *remoteHost, *remoteUser)

	client, err := gocb.Connect(*remoteHost, gocb.ClusterOptions{
		Username: *remoteUser,
		Password: *remotePass,
	})
	if err != nil {
		log.Printf("failed to connect to couchbase cluster: %s", err)
		return
	}

	err = client.WaitUntilReady(10*time.Second, nil)
	if err != nil {
		log.Printf("failed to wait for cluster to come online: %s", err)
		return
	}

	log.Printf("connected to remote host")

	topologyManager := server.NewTopologyManager(server.TopologyManagerConfig{
		LocalHostname: *localHostname,
		LocalPort:     int(*port),
	})

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()

	routing_v1.RegisterRoutingServer(s, server.NewRoutingServer(topologyManager))
	data_v1.RegisterDataServer(s, server.NewDataServer(client))
	query_v1.RegisterQueryServer(s, server.NewQueryServer(client))
	search_v1.RegisterSearchServer(s, server.NewSearchServer(client))
	analytics_v1.RegisterAnalyticsServer(s, server.NewAnalyticsServer(client))
	admin_bucket_v1.RegisterBucketAdminServer(s, server.NewBucketAdminServer(client))

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
