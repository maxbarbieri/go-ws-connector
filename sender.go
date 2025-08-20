package ws_connector

import (
	log "github.com/sirupsen/logrus"
	"sync/atomic"
)

type Sender interface {
	// SendData send a response to the associated subscriber. If last is set to true, the subscriber will not be able to receive any more data.
	SendData(data interface{}, last bool) error

	// SendError send an error to the associated subscriber. If last is set to true, the subscriber will not be able to receive any more data.
	SendError(error error, last bool) error

	Close() error
	IsUnsubscribed() bool
	GetCustomFields() Map

	GetConnector() Connector
}

type wsSender struct {
	wsConnector  *websocketConnector
	subId        uint64
	topic        string
	disabled     atomic.Bool
	customFields Map
}

func (s *wsSender) SendData(data interface{}, last bool) error {
	//allow sending a nil payload only if this is the last message (i.e.: if we want to close the subscription)
	if data == nil && !last {
		return ATTEMPT_TO_SEND_NIL_ERROR
	}

	//if this sender is enabled (the peer is still subscribed) and the connection is active
	if !s.disabled.Load() && s.wsConnector.ongoingResetLock.TryRLock() {
		defer s.wsConnector.ongoingResetLock.RUnlock()

		//just send the message to the outgoing messages handler
		s.wsConnector.outgoingWsMsgChan <- &wsSentMessage{
			Type:   subscriptionData,
			Id:     s.subId,
			Method: s.topic,
			Last:   last,
			Data:   data,
		}

		if int(s.wsConnector.outgoingWsMsgChanPrevQueue.Load()) != len(s.wsConnector.outgoingWsMsgChan) && len(s.wsConnector.outgoingWsMsgChan)%10 == 0 {
			log.Warningf("[%s][Sender.SendData] outgoingWsMsgChan queue: %d\n", s.wsConnector.logTag, len(s.wsConnector.outgoingWsMsgChan))
			s.wsConnector.outgoingWsMsgChanPrevQueue.Store(int64(len(s.wsConnector.outgoingWsMsgChan)))
		}

		if last { //if this was the last message for this subscription
			//disable this sender
			s.disable()

			//remove the subscription info from the wsConnector's map
			s.wsConnector.removeSubscriptionRequestInfo(s.subId, true)
		}

		return nil

	} else { //if the sender is disabled or the connection is not active
		return WS_CONNECTION_DOWN_ERROR
	}
}

func (s *wsSender) SendError(error error, last bool) error {
	if error == nil {
		return ATTEMPT_TO_SEND_NIL_ERROR
	}

	//if this sender is enabled (the peer is still subscribed) and the connection is active
	if !s.disabled.Load() && s.wsConnector.ongoingResetLock.TryRLock() {
		defer s.wsConnector.ongoingResetLock.RUnlock()

		//just send the message to the outgoing messages handler
		s.wsConnector.outgoingWsMsgChan <- &wsSentMessage{
			Type:   subscriptionData,
			Id:     s.subId,
			Method: s.topic,
			Error:  error.Error(),
			Last:   last,
		}

		if int(s.wsConnector.outgoingWsMsgChanPrevQueue.Load()) != len(s.wsConnector.outgoingWsMsgChan) && len(s.wsConnector.outgoingWsMsgChan)%10 == 0 {
			log.Warningf("[%s][Sender.SendError] outgoingWsMsgChan queue: %d\n", s.wsConnector.logTag, len(s.wsConnector.outgoingWsMsgChan))
			s.wsConnector.outgoingWsMsgChanPrevQueue.Store(int64(len(s.wsConnector.outgoingWsMsgChan)))
		}

		if last { //if this was the last message for this subscription
			//disable this sender
			s.disable()

			//remove the subscription info from the wsConnector's map
			s.wsConnector.removeSubscriptionRequestInfo(s.subId, true)
		}

		return nil

	} else { //if the sender is disabled or the connection is not active
		return WS_CONNECTION_DOWN_ERROR
	}
}

func (s *wsSender) Close() error {
	return s.SendData(nil, true)
}

func (s *wsSender) IsUnsubscribed() bool {
	return s.disabled.Load()
}

func (s *wsSender) disable() {
	s.disabled.Store(true)
}

func (s *wsSender) GetCustomFields() Map {
	return s.customFields
}

func (s *wsSender) GetConnector() Connector {
	return s.wsConnector
}
