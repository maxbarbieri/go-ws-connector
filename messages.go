package ws_connector

import "encoding/json"

type msgType int

const (
	request msgType = iota
	response
	subscriptionRequest
	subscriptionData
	unsubscriptionRequest
)

type wsSentMessage struct {
	Type   msgType     `json:"type"`
	Id     uint64      `json:"id,omitempty"` //used to match requests and responses (optional, a request with no id or with id = 0 does not require a response)
	Method string      `json:"method,omitempty"`
	Last   bool        `json:"last,omitempty"` //used only for subscription data messages, if true it means that this is the last response for the specified request id
	Error  string      `json:"error,omitempty"`
	Data   interface{} `json:"data"`
}

type wsReceivedMessage struct {
	Type   msgType         `json:"type"`
	Id     uint64          `json:"id,omitempty"` //used to match requests and responses (optional, a request with no id or with id = 0 does not require a response)
	Method string          `json:"method,omitempty"`
	Last   bool            `json:"last,omitempty"` //used only for subscription data messages, if true it means that this is the last response for the specified request id
	Error  string          `json:"error,omitempty"`
	Data   json.RawMessage `json:"data"`
}
