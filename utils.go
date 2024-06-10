package ws_connector

import (
	"fmt"
)

var (
	UNKNOWN_METHOD_ERROR                                = fmt.Errorf("unknown method")
	UNKNOWN_TOPIC_ERROR                                 = fmt.Errorf("unknown topic")
	ATTEMPT_TO_SEND_NIL_ERROR                           = fmt.Errorf("attempt to send nil data")
	ATTEMPT_TO_RESPOND_TO_FIRE_AND_FORGET_REQUEST_ERROR = fmt.Errorf("attempt to respond to a fire&forget request")
	ATTEMPT_TO_SEND_MULTIPLE_RESPONSES_TO_REQUEST       = fmt.Errorf("attempt to send multiple responses to a request")
	WS_CONNECTION_DOWN_ERROR                            = fmt.Errorf("ws connection down")
	DUPLICATE_REQ_ID_ERROR                              = fmt.Errorf("duplicate req id (wait for previous request with this id to be completed before reusing the id)")
	RESPONSE_CHANNEL_ALREADY_REQUESTED_ERROR            = fmt.Errorf("response channel already requested for this ResponseReader")
	REQUEST_CHANNEL_ALREADY_REQUESTED_ERROR             = fmt.Errorf("request channel already requested for this SubscriptionRequestReader")
	DATA_CHANNEL_ALREADY_REQUESTED_ERROR                = fmt.Errorf("data channel already requested for this SubscriptionDataReader")
)

type requestInfo struct {
	requestReader *RequestReader
	responder     *wsResponder
}

type subscriptionInfo struct {
	subscriptionRequestReader *SubscriptionRequestReader
	sender                    *wsSender
}

// SendRequest function that wraps the SendRequest method of a connector + the request of
// typed response channels, all in one call.
// This function sends only requests that require a response, for Fire&Forget requests
// please use the connector's SendRequest method directly.
func SendRequest[ResponseType any](conn Connector, method string, data interface{}) (chan *ResponseType, chan error, error) {
	responseReader, err := conn.SendRequest(method, data, true)
	if err != nil {
		return nil, nil, err
	}
	var typedResponseChan chan *ResponseType
	var errorChan chan error
	typedResponseChan, errorChan, err = GetTypedResponseChannels[ResponseType](responseReader)
	if err != nil {
		return nil, nil, err
	}
	return typedResponseChan, errorChan, nil
}
