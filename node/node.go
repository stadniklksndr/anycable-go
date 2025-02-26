package node

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/anycable/anycable-go/common"
	"github.com/anycable/anycable-go/hub"
	"github.com/anycable/anycable-go/metrics"
	"github.com/anycable/anycable-go/ws"
	"github.com/apex/log"
)

const (
	metricsGoroutines      = "goroutines_num"
	metricsMemSys          = "mem_sys_bytes"
	metricsClientsNum      = "clients_num"
	metricsUniqClientsNum  = "clients_uniq_num"
	metricsStreamsNum      = "broadcast_streams_num"
	metricsDisconnectQueue = "disconnect_queue_size"

	metricsFailedAuths           = "failed_auths_total"
	metricsReceivedMsg           = "client_msg_total"
	metricsFailedCommandReceived = "failed_client_msg_total"
	metricsBroadcastMsg          = "broadcast_msg_total"
	metricsUnknownBroadcast      = "failed_broadcast_msg_total"

	metricsSentMsg    = "server_msg_total"
	metricsFailedSent = "failed_server_msg_total"

	metricsDataSent     = "data_sent_total"
	metricsDataReceived = "data_rcvd_total"
)

// AppNode describes a basic node interface
type AppNode interface {
	HandlePubSub(msg []byte)
	LookupSession(id string) *Session
	Authenticate(s *Session) (*common.ConnectResult, error)
	Subscribe(s *Session, msg *common.Message) (*common.CommandResult, error)
	Unsubscribe(s *Session, msg *common.Message) (*common.CommandResult, error)
	Perform(s *Session, msg *common.Message) (*common.CommandResult, error)
	Disconnect(s *Session) error
}

// Connection represents underlying connection
type Connection interface {
	Write(msg []byte, deadline time.Time) error
	WriteBinary(msg []byte, deadline time.Time) error
	Read() ([]byte, error)
	Close(code int, reason string)
}

// Node represents the whole application
type Node struct {
	metrics metrics.Instrumenter

	config       *Config
	hub          *hub.Hub
	controller   Controller
	disconnector Disconnector
	shutdownCh   chan struct{}
	shutdownMu   sync.Mutex
	closed       bool
	log          *log.Entry
}

var _ AppNode = (*Node)(nil)

// NewNode builds new node struct
func NewNode(controller Controller, metrics *metrics.Metrics, config *Config) *Node {
	node := &Node{
		metrics:    metrics,
		config:     config,
		controller: controller,
		shutdownCh: make(chan struct{}),
		log:        log.WithFields(log.Fields{"context": "node"}),
	}

	node.hub = hub.NewHub(config.HubGopoolSize)

	if metrics != nil {
		node.registerMetrics()
	}

	return node
}

// Start runs all the required goroutines
func (n *Node) Start() error {
	go n.hub.Run()
	go n.collectStats()

	return nil
}

// SetDisconnector set disconnector for the node
func (n *Node) SetDisconnector(d Disconnector) {
	n.disconnector = d
}

// HandleCommand parses incoming message from client and
// execute the command (if recognized)
func (n *Node) HandleCommand(s *Session, msg *common.Message) (err error) {
	s.Log.Debugf("Incoming message: %v", msg)
	switch msg.Command {
	case "subscribe":
		_, err = n.Subscribe(s, msg)
	case "unsubscribe":
		_, err = n.Unsubscribe(s, msg)
	case "message":
		_, err = n.Perform(s, msg)
	default:
		err = fmt.Errorf("Unknown command: %s", msg.Command)
	}

	return
}

// HandlePubSub parses incoming pubsub message and broadcast it
func (n *Node) HandlePubSub(raw []byte) {
	msg, err := common.PubSubMessageFromJSON(raw)

	if err != nil {
		n.metrics.CounterIncrement(metricsUnknownBroadcast)
		n.log.Warnf("Failed to parse pubsub message '%s' with error: %v", raw, err)
		return
	}

	switch v := msg.(type) {
	case common.StreamMessage:
		n.Broadcast(&v)
	case common.RemoteDisconnectMessage:
		n.RemoteDisconnect(&v)
	}
}

func (n *Node) LookupSession(id string) *Session {
	hubSession := n.hub.FindByIdentifier(id)
	session, _ := hubSession.(*Session)
	return session
}

// Shutdown stops all services (hub, controller)
func (n *Node) Shutdown() (err error) {
	n.shutdownMu.Lock()
	if n.closed {
		n.shutdownMu.Unlock()
		return errors.New("Already shut down")
	}

	close(n.shutdownCh)

	n.closed = true
	n.shutdownMu.Unlock()

	if n.hub != nil {
		n.hub.Shutdown()

		active := n.hub.Size()

		if active > 0 {
			n.log.Infof("Closing active connections: %d", active)
			disconnectMessage := common.NewDisconnectMessage(common.SERVER_RESTART_REASON, true)

			n.hub.DisconnectSesssions(disconnectMessage, common.SERVER_RESTART_REASON)

			n.log.Info("All active connections closed")

			// Wait to make sure that disconnect queue is not empty
			time.Sleep(time.Second)
		}
	}

	if n.disconnector != nil {
		err := n.disconnector.Shutdown()

		if err != nil {
			n.log.Warnf("%v", err)
		}
	}

	if n.controller != nil {
		err := n.controller.Shutdown()

		if err != nil {
			n.log.Warnf("%v", err)
		}
	}

	return
}

// Authenticate calls controller to perform authentication.
// If authentication is successful, session is registered with a hub.
func (n *Node) Authenticate(s *Session) (res *common.ConnectResult, err error) {
	res, err = n.controller.Authenticate(s.GetID(), s.env)

	if err != nil {
		s.Disconnect("Auth Error", ws.CloseInternalServerErr)
		return
	}

	if res.Status == common.SUCCESS {
		s.SetIdentifiers(res.Identifier)
		s.Connected = true

		n.hub.AddSession(s)
	} else {
		if res.Status == common.FAILURE {
			n.metrics.CounterIncrement(metricsFailedAuths)
		}

		defer s.Disconnect("Auth Failed", ws.CloseNormalClosure)
	}

	n.handleCallReply(s, res.ToCallResult())

	return
}

// Subscribe subscribes session to a channel
func (n *Node) Subscribe(s *Session, msg *common.Message) (res *common.CommandResult, err error) {
	s.smu.Lock()

	if ok := s.subscriptions.HasChannel(msg.Identifier); ok {
		s.smu.Unlock()
		err = fmt.Errorf("Already subscribed to %s", msg.Identifier)
		return
	}

	res, err = n.controller.Subscribe(s.GetID(), s.env, s.GetIdentifiers(), msg.Identifier)

	if err != nil {
		if res == nil || res.Status == common.ERROR {
			s.Log.Errorf("Subscribe error: %v", err)
		}
	} else {
		s.subscriptions.AddChannel(msg.Identifier)
		s.Log.Debugf("Subscribed to channel: %s", msg.Identifier)
	}

	s.smu.Unlock()

	if res != nil {
		n.handleCommandReply(s, msg, res)
	}

	return
}

// Unsubscribe unsubscribes session from a channel
func (n *Node) Unsubscribe(s *Session, msg *common.Message) (res *common.CommandResult, err error) {
	s.smu.Lock()

	if ok := s.subscriptions.HasChannel(msg.Identifier); !ok {
		s.smu.Unlock()
		err = fmt.Errorf("Unknown subscription %s", msg.Identifier)
		return
	}

	res, err = n.controller.Unsubscribe(s.GetID(), s.env, s.GetIdentifiers(), msg.Identifier)

	if err != nil {
		if res == nil || res.Status == common.ERROR {
			s.Log.Errorf("Unsubscribe error: %v", err)
		}
	} else {
		// Make sure to remove all streams subscriptions
		res.StopAllStreams = true

		s.subscriptions.RemoveChannel(msg.Identifier)

		s.Log.Debugf("Unsubscribed from channel: %s", msg.Identifier)
	}

	s.smu.Unlock()

	if res != nil {
		n.handleCommandReply(s, msg, res)
	}

	return
}

// Perform executes client command
func (n *Node) Perform(s *Session, msg *common.Message) (res *common.CommandResult, err error) {
	s.smu.Lock()

	if ok := s.subscriptions.HasChannel(msg.Identifier); !ok {
		s.smu.Unlock()
		err = fmt.Errorf("Unknown subscription %s", msg.Identifier)
		return
	}

	s.smu.Unlock()

	data, ok := msg.Data.(string)

	if !ok {
		err = fmt.Errorf("Perform data must be a string, got %v", msg.Data)
		return
	}

	res, err = n.controller.Perform(s.GetID(), s.env, s.GetIdentifiers(), msg.Identifier, data)

	if err != nil {
		if res == nil || res.Status == common.ERROR {
			s.Log.Errorf("Perform error: %v", err)
		}
	} else {
		s.Log.Debugf("Perform result: %v", res)
	}

	if res != nil {
		n.handleCommandReply(s, msg, res)
	}

	return
}

// Broadcast message to stream
func (n *Node) Broadcast(msg *common.StreamMessage) {
	n.metrics.CounterIncrement(metricsBroadcastMsg)
	n.log.Debugf("Incoming pubsub message: %v", msg)
	n.hub.BroadcastMessage(msg)
}

// Disconnect adds session to disconnector queue and unregister session from hub
func (n *Node) Disconnect(s *Session) error {
	n.hub.RemoveSessionLater(s)
	return n.disconnector.Enqueue(s)
}

// DisconnectNow execute disconnect on controller
func (n *Node) DisconnectNow(s *Session) error {
	sessionSubscriptions := s.subscriptions.Channels()

	ids := s.GetIdentifiers()

	s.Log.Debugf("Disconnect %s %s %v %v", ids, s.env.URL, s.env.Headers, sessionSubscriptions)

	err := n.controller.Disconnect(
		s.GetID(),
		s.env,
		ids,
		sessionSubscriptions,
	)

	if err != nil {
		s.Log.Errorf("Disconnect error: %v", err)
	}

	return err
}

// RemoteDisconnect find a session by identifier and closes it
func (n *Node) RemoteDisconnect(msg *common.RemoteDisconnectMessage) {
	n.metrics.CounterIncrement(metricsBroadcastMsg)
	n.log.Debugf("Incoming pubsub command: %v", msg)
	n.hub.RemoteDisconnect(msg)
}

func transmit(s *Session, transmissions []string) {
	for _, msg := range transmissions {
		s.SendJSONTransmission(msg)
	}
}

func (n *Node) handleCommandReply(s *Session, msg *common.Message, reply *common.CommandResult) bool {
	// Returns true if any of the subscriptions/channel/connections state has changed
	isDirty := false

	if reply.Disconnect {
		defer s.Disconnect("Command Failed", ws.CloseAbnormalClosure)
	}

	uid := s.GetID()

	if reply.StopAllStreams {
		n.hub.UnsubscribeSessionFromChannel(uid, msg.Identifier)
		s.subscriptions.RemoveChannelStreams(msg.Identifier)
	} else if reply.StoppedStreams != nil {
		isDirty = true

		for _, stream := range reply.StoppedStreams {
			n.hub.UnsubscribeSession(uid, stream, msg.Identifier)
			s.subscriptions.RemoveChannelStream(msg.Identifier, stream)
		}
	}

	if reply.Streams != nil {
		isDirty = true

		for _, stream := range reply.Streams {
			n.hub.SubscribeSession(uid, stream, msg.Identifier)
			s.subscriptions.AddChannelStream(msg.Identifier, stream)
		}
	}

	if reply.IState != nil {
		isDirty = true

		s.smu.Lock()
		s.env.MergeChannelState(msg.Identifier, &reply.IState)
		s.smu.Unlock()
	}

	isConnectionDirty := n.handleCallReply(s, reply.ToCallResult())
	return isDirty || isConnectionDirty
}

func (n *Node) handleCallReply(s *Session, reply *common.CallResult) bool {
	isDirty := false

	if reply.CState != nil {
		isDirty = true

		s.smu.Lock()
		s.env.MergeConnectionState(&reply.CState)
		s.smu.Unlock()
	}

	if reply.Broadcasts != nil {
		for _, broadcast := range reply.Broadcasts {
			n.Broadcast(broadcast)
		}
	}

	if reply.Transmissions != nil {
		transmit(s, reply.Transmissions)
	}

	return isDirty
}

func (n *Node) collectStats() {
	statsCollectInterval := time.Duration(n.config.StatsRefreshInterval) * time.Second

	for {
		select {
		case <-n.shutdownCh:
			return
		case <-time.After(statsCollectInterval):
			n.collectStatsOnce()
		}
	}
}

func (n *Node) collectStatsOnce() {
	n.metrics.GaugeSet(metricsGoroutines, uint64(runtime.NumGoroutine()))

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	n.metrics.GaugeSet(metricsMemSys, m.Sys)

	n.metrics.GaugeSet(metricsClientsNum, uint64(n.hub.Size()))
	n.metrics.GaugeSet(metricsUniqClientsNum, uint64(n.hub.UniqSize()))
	n.metrics.GaugeSet(metricsStreamsNum, uint64(n.hub.StreamsSize()))
	n.metrics.GaugeSet(metricsDisconnectQueue, uint64(n.disconnector.Size()))
}

func (n *Node) registerMetrics() {
	n.metrics.RegisterGauge(metricsGoroutines, "The number of Go routines")
	n.metrics.RegisterGauge(metricsMemSys, "The total bytes of memory obtained from the OS")

	n.metrics.RegisterGauge(metricsClientsNum, "The number of active clients")
	n.metrics.RegisterGauge(metricsUniqClientsNum, "The number of unique clients (with respect to connection identifiers)")
	n.metrics.RegisterGauge(metricsStreamsNum, "The number of active broadcasting streams")
	n.metrics.RegisterGauge(metricsDisconnectQueue, "The size of delayed disconnect")

	n.metrics.RegisterCounter(metricsFailedAuths, "The total number of failed authentication attempts")
	n.metrics.RegisterCounter(metricsReceivedMsg, "The total number of received messages from clients")
	n.metrics.RegisterCounter(metricsFailedCommandReceived, "The total number of unrecognized messages received from clients")
	n.metrics.RegisterCounter(metricsBroadcastMsg, "The total number of messages received through PubSub (for broadcast)")
	n.metrics.RegisterCounter(metricsUnknownBroadcast, "The total number of unrecognized messages received through PubSub")

	n.metrics.RegisterCounter(metricsSentMsg, "The total number of messages sent to clients")
	n.metrics.RegisterCounter(metricsFailedSent, "The total number of messages failed to send to clients")

	n.metrics.RegisterCounter(metricsDataSent, "The total amount of bytes sent to clients")
	n.metrics.RegisterCounter(metricsDataReceived, "The total amount of bytes received from clients")
}
