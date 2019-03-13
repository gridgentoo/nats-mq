package core

import (
	"fmt"
	"sync"
	"time"

	"github.com/ibm-messaging/mq-golang/ibmmq"
	"github.com/nats-io/go-nats"
	"github.com/nats-io/nats-mq/server/conf"
)

// NATS2QueueConnector connects a NATS subject to an MQ queue
type NATS2QueueConnector struct {
	sync.Mutex

	config conf.ConnectorConfig
	bridge Bridge

	qMgr  *ibmmq.MQQueueManager
	queue *ibmmq.MQObject

	sub *nats.Subscription

	stats ConnectorStats
}

// NewNATS2QueueConnector create a nats to MQ connector
func NewNATS2QueueConnector(bridge Bridge, config conf.ConnectorConfig) Connector {
	return &NATS2QueueConnector{
		config: config,
		bridge: bridge,
		stats:  NewConnectorStats(),
	}
}

func (mq *NATS2QueueConnector) String() string {
	return fmt.Sprintf("NATS:%s to Queue:%s", mq.config.Subject, mq.config.Queue)
}

// Stats returns a copy of the current stats for this connector
func (mq *NATS2QueueConnector) Stats() ConnectorStats {
	mq.Lock()
	defer mq.Unlock()
	return mq.stats
}

// Config returns the configuraiton for this connector
func (mq *NATS2QueueConnector) Config() conf.ConnectorConfig {
	return mq.config
}

// Start the connector
func (mq *NATS2QueueConnector) Start() error {
	mq.Lock()
	defer mq.Unlock()

	mq.stats.Name = mq.String()

	if mq.bridge.NATS() == nil {
		return fmt.Errorf("%s connector requires nats to be available", mq.String())
	}

	mqconfig := mq.config.MQ
	queueName := mq.config.Queue

	mq.bridge.Logger().Tracef("starting connection %s", mq.String())

	qMgr, err := ConnectToQueueManager(mqconfig)
	if err != nil {
		return err
	}

	mq.bridge.Logger().Tracef("connected to queue manager %s at %s as %s for %s", mqconfig.QueueManager, mqconfig.ConnectionName, mqconfig.ChannelName, mq.String())

	mq.qMgr = qMgr

	// Create the Object Descriptor that allows us to give the queue name
	mqod := ibmmq.NewMQOD()
	openOptions := ibmmq.MQOO_OUTPUT
	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = queueName

	qObject, err := mq.qMgr.Open(mqod, openOptions)

	if err != nil {
		return err
	}

	mq.queue = &qObject

	sub, err := mq.bridge.NATS().Subscribe(mq.config.Subject, mq.messageHandler)

	if err != nil {
		return err
	}

	mq.sub = sub

	mq.stats.AddConnect()
	mq.bridge.Logger().Tracef("opened and reading %s", queueName)
	mq.bridge.Logger().Noticef("started connection %s", mq.String())

	return nil
}

func (mq *NATS2QueueConnector) messageHandler(m *nats.Msg) {
	mq.Lock()
	defer mq.Unlock()
	start := time.Now()

	qmgrFlag := mq.qMgr

	if mq.config.ExcludeHeaders {
		qmgrFlag = nil
	}

	mq.stats.AddMessageIn(int64(len(m.Data)))
	mqmd, handle, buffer, err := mq.bridge.NATSToMQMessage(m.Data, m.Reply, qmgrFlag)

	pmo := ibmmq.NewMQPMO()
	pmo.Options = ibmmq.MQPMO_NO_SYNCPOINT
	pmo.OriginalMsgHandle = handle

	// Now put the message to the queue
	err = mq.queue.Put(mqmd, pmo, buffer)

	if err != nil {
		mq.bridge.Logger().Noticef("MQ publish failure, %s, %s", mq.String(), err.Error())
	} else {
		mq.stats.AddMessageOut(int64(len(buffer)))
		mq.stats.AddRequestTime(time.Now().Sub(start))
	}
}

// Shutdown the connector
func (mq *NATS2QueueConnector) Shutdown() error {
	mq.Lock()
	defer mq.Unlock()
	mq.stats.AddDisconnect()

	mq.bridge.Logger().Noticef("shutting down connection %s", mq.String())

	var err error

	queue := mq.queue
	mq.queue = nil

	if queue != nil {
		err = queue.Close(0)
	}

	if mq.qMgr != nil {
		_ = mq.qMgr.Disc()
		mq.qMgr = nil
		mq.bridge.Logger().Tracef("disconnected from queue manager for %s", mq.String())
	}

	if mq.sub != nil {
		mq.sub.Unsubscribe()
		mq.sub = nil
	}

	return err // ignore the disconnect error
}