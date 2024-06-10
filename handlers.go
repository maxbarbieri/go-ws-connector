package ws_connector

// RequestHandlerFunction if the pointer to the responder object is nil, it means that the request sender did not require any responses;
// handler functions should check this before using the responder.
// Use the function GetTypedRequestData to get the request data from the reader.
type RequestHandlerFunction func(responder Responder, requestReader *RequestReader)

type RequestHandlerInfo struct {
	Method  string
	Handler RequestHandlerFunction
}

// SubscriptionRequestHandlerFunction Use the function GetTypedSubscriptionRequestChannels to get the channels from which the initial subscription request data (and its updates) can be read.
// When sending data from the handler to the subscriber, the `last` flag should be set to true only for the last
// message of the stream. After sending the last message, no further messages should be sent, as they are ignored by
// the peer's WebsocketConnector.
// Peers can unsubscribe from a request they previously sent, if this happens the method `sender.IsPeerUnsubscribed()`
// will return true, so check it in your handler if you want to know whether a peer has unsubscribed.
type SubscriptionRequestHandlerFunction func(sender Sender, subscriptionRequestReader *SubscriptionRequestReader)

type SubscriptionRequestHandlerInfo struct {
	Topic   string
	Handler SubscriptionRequestHandlerFunction
}
