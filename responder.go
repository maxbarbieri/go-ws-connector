package ws_connector

import (
	"sync/atomic"
)

type Responder interface {
	// SendResponse sends a response (for the request to which this responder is attached) to the peer on the other side of the websocket connection.
	SendResponse(data interface{}) error

	// SendError sends an error response (for the request to which this responder is attached) to the peer on the other side of the websocket connection.
	SendError(error error) error

	// GetConnector get a reference to the parent Connector object.
	GetConnector() Connector

	// ResponseRequired returns true if the request sender wants a response, false if this is a fire&forget request.
	ResponseRequired() bool
}

type wsResponder struct {
	wsConnector *websocketConnector
	reqId       uint64
	method      string
	disabled    atomic.Bool
}

func (r *wsResponder) SendResponse(data interface{}) error {
	if r.reqId == 0 {
		return ATTEMPT_TO_RESPOND_TO_FIRE_AND_FORGET_REQUEST_ERROR
	}

	if data == nil {
		return ATTEMPT_TO_SEND_NIL_ERROR
	}

	//if this responder is enabled (the peer is still subscribed) and the connection is active
	if !r.disabled.Load() {
		if r.wsConnector.ongoingResetLock.TryRLock() {
			defer r.wsConnector.ongoingResetLock.RUnlock()

			//just send the message to the outgoing messages handler
			r.wsConnector.outgoingWsMsgChan <- &wsSentMessage{
				Type:   response,
				Id:     r.reqId,
				Method: r.method,
				Data:   data,
			}

			//remove the request info from the wsConnector's map
			r.wsConnector.removeRequestInfo(r.reqId, true)

			//disable the responder
			r.disable()

			return nil

		} else { //if the connection is not active
			return WS_CONNECTION_DOWN_ERROR
		}

	} else { //if the responder is disabled
		return ATTEMPT_TO_SEND_MULTIPLE_RESPONSES_TO_REQUEST
	}
}

func (r *wsResponder) SendError(error error) error {
	if r.reqId == 0 {
		return ATTEMPT_TO_RESPOND_TO_FIRE_AND_FORGET_REQUEST_ERROR
	}

	if error == nil {
		return ATTEMPT_TO_SEND_NIL_ERROR
	}

	//if this responder is enabled (the peer is still subscribed) and the connection is active
	if !r.disabled.Load() {
		if r.wsConnector.ongoingResetLock.TryRLock() {
			defer r.wsConnector.ongoingResetLock.RUnlock()

			//just send the message to the outgoing messages handler
			r.wsConnector.outgoingWsMsgChan <- &wsSentMessage{
				Type:   response,
				Id:     r.reqId,
				Method: r.method,
				Error:  error.Error(),
			}

			//remove the request info from the wsConnector's map
			r.wsConnector.removeRequestInfo(r.reqId, true)

			//disable the responder
			r.disable()

			return nil

		} else { //if the connection is not active
			return WS_CONNECTION_DOWN_ERROR
		}

	} else { //if the responder is disabled
		return ATTEMPT_TO_SEND_MULTIPLE_RESPONSES_TO_REQUEST
	}
}

func (r *wsResponder) disable() {
	r.disabled.Store(true)
}

func (r *wsResponder) GetConnector() Connector {
	return r.wsConnector
}

func (r *wsResponder) ResponseRequired() bool {
	return r.reqId != 0
}
