package core

import (
	"testing"
	"time"

	"github.com/ibm-messaging/mq-golang/ibmmq"
	stan "github.com/nats-io/go-nats-streaming"
	"github.com/nats-io/nats-mq/message"
	"github.com/nats-io/nats-mq/server/conf"
	"github.com/stretchr/testify/require"
)

func TestSimpleSendOnTopicReceiveOnStan(t *testing.T) {
	channel := "test"
	topic := "dev/"
	msg := "hello world"

	connect := []conf.ConnectorConfig{
		conf.ConnectorConfig{
			Type:           "Topic2Stan",
			Channel:        channel,
			Topic:          topic,
			ExcludeHeaders: true,
		},
	}

	tbs, err := StartTestEnvironment(connect)
	require.NoError(t, err)
	defer tbs.Close()

	done := make(chan string)

	sub, err := tbs.SC.Subscribe(channel, func(msg *stan.Msg) {
		done <- string(msg.Data)
	})
	defer sub.Unsubscribe()

	err = tbs.PutMessageOnTopic(topic, ibmmq.NewMQMD(), []byte(msg))
	require.NoError(t, err)

	timer := time.NewTimer(3 * time.Second)
	go func() {
		<-timer.C
		done <- ""
	}()

	received := <-done
	require.Equal(t, msg, received)
}

func TestSendOnTopicReceiveOnStanMQMD(t *testing.T) {
	start := time.Now().UTC()
	channel := "test"
	topic := "dev/"
	msg := "hello world"

	connect := []conf.ConnectorConfig{
		conf.ConnectorConfig{
			Type:           "Topic2Stan",
			Channel:        channel,
			Topic:          topic,
			ExcludeHeaders: false,
		},
	}

	tbs, err := StartTestEnvironment(connect)
	require.NoError(t, err)
	defer tbs.Close()

	done := make(chan []byte)

	sub, err := tbs.SC.Subscribe(channel, func(msg *stan.Msg) {
		done <- msg.Data
	})
	defer sub.Unsubscribe()

	mqmd := ibmmq.NewMQMD()
	err = tbs.PutMessageOnTopic(topic, mqmd, []byte(msg))
	require.NoError(t, err)

	// don't wait forever
	timer := time.NewTimer(3 * time.Second)
	go func() {
		<-timer.C
		done <- []byte{}
	}()

	received := <-done

	require.True(t, len(received) > 0)

	bridgeMessage, err := message.DecodeBridgeMessage(received)
	require.NoError(t, err)

	require.Equal(t, msg, string(bridgeMessage.Body))
	require.Equal(t, start.Format("20060102"), bridgeMessage.Header.PutDate)
	require.True(t, start.Format("15040500") < bridgeMessage.Header.PutTime)
}