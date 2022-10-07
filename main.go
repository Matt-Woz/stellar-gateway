package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/couchbase/stellar-nebula/common/topology"
	"github.com/couchbase/stellar-nebula/gateway"
	"github.com/couchbase/stellar-nebula/legacyproxy"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/couchbase/gocb/v2"
	etcd "go.etcd.io/etcd/client/v3"
)

var cbHost = flag.String("cb-host", "couchbase://127.0.0.1", "the couchbase cluster to link to")
var cbUser = flag.String("cb-user", "Administrator", "the username to use for the couchbase cluster")
var cbPass = flag.String("cb-pass", "password", "the password to use for the couchbase cluster")
var etcdHost = flag.String("etcd-host", "localhost:2379", "the etcd host to connect to")
var bindAddr = flag.String("bind-addr", "0.0.0.0", "the address to bind")
var bindPort = flag.Int("bind-port", 18098, "the port to bind to")
var advertiseAddr = flag.String("advertise-addr", "127.0.0.1", "the address to use when advertising this node")
var advertisePort = flag.Uint64("advertise-port", 18098, "the port to use when advertising this node")
var nodeID = flag.String("node-id", "", "the local node id for this service")
var serverGroup = flag.String("server-group", "", "the local hostname for this service")

func main() {
	flag.Parse()

	// NodeID must not be blank, so lets generate a unique UUID if one wasn't provided...
	if nodeID == nil || *nodeID == "" {
		genNodeID := uuid.NewString()
		nodeID = &genNodeID
	}

	// initialize the logger
	logLevel := zap.NewAtomicLevel()
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(config)
	consoleEncoder := zapcore.NewConsoleEncoder(config)
	logFile, _ := os.OpenFile("text.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, logLevel),
		zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), logLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// switch to debug level logs for ... debugging

	logLevel.SetLevel(zap.DebugLevel)

	// start connecting to the underlying cluster
	log.Printf("linking to couchbase cluster at: %s (user: %s)", *cbHost, *cbUser)

	client, err := gocb.Connect(*cbHost, gocb.ClusterOptions{
		Username: *cbUser,
		Password: *cbPass,
	})
	if err != nil {
		log.Printf("failed to connect to couchbase cluster: %s", err)
		os.Exit(1)
	}

	err = client.WaitUntilReady(10*time.Second, nil)
	if err != nil {
		log.Printf("failed to wait for couchbase cluster connection: %s", err)
		os.Exit(1)
	}

	log.Printf("connected to couchbase cluster")

	log.Printf("connect to etcd instance at: %s", *etcdHost)

	etcdClient, err := etcd.New(etcd.Config{
		Endpoints:   []string{*etcdHost},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Printf("failed to connect to etcd: %s", err)
		os.Exit(1)
	}

	topologyProvider, err := topology.NewEtcdProvider(topology.EtcdProviderOptions{
		EtcdClient: etcdClient,
		KeyPrefix:  "/nebula/topology",
	})
	if err != nil {
		log.Printf("failed to initialize topology provider: %s", err)
		os.Exit(1)
	}

	// join the cluster topology
	log.Printf("joining nebula cluster toplogy")
	topologyProvider.Join(&topology.Endpoint{
		NodeID:        *nodeID,
		AdvertiseAddr: *advertiseAddr,
		AdvertisePort: int(*advertisePort),
		ServerGroup:   *serverGroup,
	})

	// setup the gateway server
	log.Printf("initializing gateway system")
	gateway, err := gateway.NewGateway(&gateway.GatewayOptions{
		Logger:           logger,
		BindAddress:      *bindAddr,
		BindPort:         *bindPort,
		TopologyProvider: topologyProvider,
		CbClient:         client,
	})
	if err != nil {
		log.Fatalf("failed to initialize gateway: %s", err)
	}

	waitCh := make(chan struct{})

	go func() {
		// start serving requests
		log.Printf("starting to serve grpc")
		err := gateway.Run(context.Background())
		if err != nil {
			log.Fatalf("failed to run gateway: %v", err)
		}

		waitCh <- struct{}{}
	}()

	log.Printf("starting to serve legacy")
	lproxy, err := legacyproxy.NewSystem(&legacyproxy.SystemOptions{
		Logger: logger,

		BindAddress: "",
		BindPorts: legacyproxy.ServicePorts{
			Mgmt: 8091,
			Kv:   11210,
		},
		TLSBindPorts: legacyproxy.ServicePorts{},

		DataServer:    gateway.DataV1Server,
		QueryServer:   gateway.QueryV1Server,
		RoutingServer: gateway.RoutingV1Server,
	})
	if err != nil {
		log.Printf("error creating legacy proxy: %s", err)
	}

	lproxy.Test()

	<-waitCh
}
