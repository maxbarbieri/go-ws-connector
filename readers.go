package ws_connector

import (
	"encoding/json"
	"fmt"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
)

/*
	RequestReader
*/

type RequestReader struct {
	reqData json.RawMessage
}

func (rr *RequestReader) GetRawRequestData() json.RawMessage {
	return rr.reqData
}

func GetTypedRequestData[RequestType any](rr *RequestReader) (*RequestType, error) {
	var obj RequestType
	err := jsoniter.ConfigFastest.Unmarshal(rr.reqData, &obj)
	if err != nil {
		return nil, fmt.Errorf("error in jsoniter.Unmarshal(rr.reqData, &obj): %s", err)
	}
	return &obj, nil
}

/*
	SubscriptionRequestReader
*/

type SubscriptionRequestReader struct {
	subscriptionRequestDataChan            chan json.RawMessage
	channelsRequested                      bool
	typedSubscriptionRequestChanBufferSize int
	errorChan                              chan error
	channelsClosed                         bool
	lock                                   sync.Mutex

	reqDataChanPrevQueue int
	errChanPrevQueue     atomic.Int64
}

func (srr *SubscriptionRequestReader) closeChannels() {
	srr.lock.Lock()
	defer srr.lock.Unlock()

	if !srr.channelsClosed {
		close(srr.subscriptionRequestDataChan)
		close(srr.errorChan)
		srr.channelsClosed = true
	}
}

func (srr *SubscriptionRequestReader) GetRawSubscriptionRequestChannels() (chan json.RawMessage, chan error, error) {
	//use locks to avoid concurrent access to the SubscriptionRequestReader
	srr.lock.Lock()
	defer srr.lock.Unlock()

	if srr.channelsRequested { //if channels for this reader have already been requested
		return nil, nil, REQUEST_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	srr.channelsRequested = true

	return srr.subscriptionRequestDataChan, srr.errorChan, nil
}

func GetTypedSubscriptionRequestChannels[SubscriptionRequestType any](srr *SubscriptionRequestReader) (chan *SubscriptionRequestType, chan error, error) {
	//use locks to avoid concurrent access to the SubscriptionRequestReader
	srr.lock.Lock()
	defer srr.lock.Unlock()

	if srr.channelsRequested { //if channels for this reader have already been requested
		return nil, nil, REQUEST_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	srr.channelsRequested = true

	//create typed subscription requests channel
	typedChan := make(chan *SubscriptionRequestType, srr.typedSubscriptionRequestChanBufferSize)

	//create a goroutine that "translates" all incoming subscription requests
	go func() {
		var typedChanPrevQueue int
		for {
			subReqData, chanOpen := <-srr.subscriptionRequestDataChan
			if chanOpen {
				var obj SubscriptionRequestType
				err := jsoniter.ConfigFastest.Unmarshal(subReqData, &obj)
				if err == nil { //no error
					typedChan <- &obj

					if typedChanPrevQueue != len(typedChan) && len(typedChan)%10 == 0 {
						log.Warningf("[GetTypedSubscriptionRequestChannels] typedChan queue: %d\n", len(typedChan))
						typedChanPrevQueue = len(typedChan)
					}

				} else { //error
					//we need the lock to correctly check whether the source error channel is closed or not
					//(note that this is a separate goroutine, not the same one that called GetTypedResponseChannels)
					srr.lock.Lock()
					if !srr.channelsClosed {
						srr.errorChan <- fmt.Errorf("error in jsoniter.Unmarshal(subReqData, &obj): %s", err)

						if int(srr.errChanPrevQueue.Load()) != len(srr.errorChan) && len(srr.errorChan)%10 == 0 {
							log.Warningf("[GetTypedSubscriptionRequestChannels] srr.errorChan queue: %d\n", len(srr.errorChan))
							srr.errChanPrevQueue.Store(int64(len(srr.errorChan)))
						}
					}
					srr.lock.Unlock()
				}

			} else { //if requests channel is closed
				close(typedChan) //close the typed requests channel too
				return           //kill this goroutine
			}
			//the returned error channel is the same channel that is directly contained in the SubscriptionRequestReader, so we don't need to handle it here
		}
	}()

	return typedChan, srr.errorChan, nil
}

/*
	ResponseReader
*/

type ResponseReader struct {
	connectorLogTag   string
	method            string
	responseChan      chan json.RawMessage
	errorChan         chan error
	channelsRequested bool
	channelsClosed    bool
	lock              sync.Mutex

	resChanPrevQueue int
	errChanPrevQueue atomic.Int64
}

func (rr *ResponseReader) closeChannels() {
	rr.lock.Lock()
	defer rr.lock.Unlock()

	if !rr.channelsClosed {
		close(rr.responseChan)
		close(rr.errorChan)
		rr.channelsClosed = true
	}
}

// GetRawResponseChannels note that both channels returned by this function are closed by the WsConnector as soon as they're not needed anymore, handle channel open flag accordingly!
func (rr *ResponseReader) GetRawResponseChannels() (chan json.RawMessage, chan error, error) {
	//use locks to avoid concurrent access to the ResponseReader (from different goroutines calling GetTypedResponseChannels at the same time)
	rr.lock.Lock()
	defer rr.lock.Unlock()

	if rr.channelsRequested { //if channels for this reader have already been requested
		return nil, nil, RESPONSE_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	rr.channelsRequested = true

	return rr.responseChan, rr.errorChan, nil
}

// GetTypedResponseOnChannels does NOT automatically close the channels passed as parameters after the response has been received
func GetTypedResponseOnChannels[ResponseType any](rr *ResponseReader, typedResponseChan chan *ResponseType, errorChan chan error) error {
	if typedResponseChan == nil {
		return fmt.Errorf("typedResponseChan can't be nil")
	}
	if errorChan == nil {
		return fmt.Errorf("errorChan can't be nil")
	}

	//use locks to avoid concurrent access to the ResponseReader (from different goroutines calling GetTypedResponseChannels at the same time)
	rr.lock.Lock()
	defer rr.lock.Unlock()

	if rr.channelsRequested { //if channels for this reader have already been requested
		return RESPONSE_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	rr.channelsRequested = true

	//create two goroutines that "translate" the incoming responses and errors

	var errChanPrevQueue atomic.Int64

	go func() {
		//avoid crashing the entire process if the specified channels are closed when writing to it
		defer func() {
			if r := recover(); r != nil {
				log.Warningf("[%s][GetTypedResponseOnChannels][dataChan goroutine] recovered from panic: %+v\n", rr.connectorLogTag, r)
			}
		}()

		var rawJsonResponse json.RawMessage
		var chanOpen bool
		var typedChanPrevQueue int
		for {
			rawJsonResponse, chanOpen = <-rr.responseChan
			if chanOpen {
				var obj ResponseType
				err := jsoniter.ConfigFastest.Unmarshal(rawJsonResponse, &obj)
				if err == nil { //no error
					typedResponseChan <- &obj

					if typedChanPrevQueue != len(typedResponseChan) && len(typedResponseChan)%10 == 0 {
						log.Warningf("[GetTypedResponseOnChannels] typedResponseChan queue: %d | method: %s\n", len(typedResponseChan), rr.method)
						typedChanPrevQueue = len(typedResponseChan)
					}

				} else { //error
					errorChan <- fmt.Errorf("error in jsoniter.Unmarshal(rawJsonResponse, &obj): %s", err)

					if int(errChanPrevQueue.Load()) != len(errorChan) && len(errorChan)%10 == 0 {
						log.Warningf("[GetTypedResponseOnChannels] errorChan queue: %d | method: %s\n", len(errorChan), rr.method)
						errChanPrevQueue.Store(int64(len(errorChan)))
					}
				}

			} else { //if response channel is closed
				return //kill this goroutine
			}
		}
	}()

	go func() {
		//avoid crashing the entire process if the specified channels are closed when writing to it
		defer func() {
			if r := recover(); r != nil {
				log.Warningf("[%s][GetTypedResponseOnChannels][errorChan goroutine] recovered from panic: %+v\n", rr.connectorLogTag, r)
			}
		}()

		var err error
		var chanOpen bool
		for {
			err, chanOpen = <-rr.errorChan
			if chanOpen {
				errorChan <- err

				if int(errChanPrevQueue.Load()) != len(errorChan) && len(errorChan)%10 == 0 {
					log.Warningf("[GetTypedResponseOnChannels] errorChan queue: %d | method: %s\n", len(errorChan), rr.method)
					errChanPrevQueue.Store(int64(len(errorChan)))
				}

			} else { //if error channel is closed
				return //kill this goroutine
			}
		}
	}()

	return nil
}

// GetTypedResponseChannels DOES automatically close the returned channels after the response has been received
func GetTypedResponseChannels[ResponseType any](rr *ResponseReader) (chan *ResponseType, chan error, error) {
	//use locks to avoid concurrent access to the ResponseReader (from different goroutines calling GetTypedResponseChannels at the same time)
	rr.lock.Lock()
	defer rr.lock.Unlock()

	if rr.channelsRequested { //if channels for this reader have already been requested
		return nil, nil, RESPONSE_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	rr.channelsRequested = true

	//create typed response channels
	typedResponseChan := make(chan *ResponseType, 1)

	//create a goroutine that "translates" the incoming responses
	go func() {
		var typedChanPrevQueue int
		for {
			rawJsonResponse, chanOpen := <-rr.responseChan
			if chanOpen {
				var obj ResponseType
				err := jsoniter.ConfigFastest.Unmarshal(rawJsonResponse, &obj)
				if err == nil { //no error
					typedResponseChan <- &obj

					if typedChanPrevQueue != len(typedResponseChan) && len(typedResponseChan)%10 == 0 {
						log.Warningf("[GetTypedResponseChannels] typedResponseChan queue: %d | method: %s\n", len(typedResponseChan), rr.method)
						typedChanPrevQueue = len(typedResponseChan)
					}

				} else { //error
					//we need the lock to correctly check whether the source error channel is closed or not
					//(note that this is a separate goroutine, not the same one that called GetTypedResponseChannels)
					rr.lock.Lock()
					if !rr.channelsClosed {
						rr.errorChan <- fmt.Errorf("error in jsoniter.Unmarshal(rawJsonResponse, &obj): %s", err)

						if int(rr.errChanPrevQueue.Load()) != len(rr.errorChan) && len(rr.errorChan)%10 == 0 {
							log.Warningf("[GetTypedResponseChannels] rr.errorChan queue: %d | method: %s\n", len(rr.errorChan), rr.method)
							rr.errChanPrevQueue.Store(int64(len(rr.errorChan)))
						}
					}
					rr.lock.Unlock()
				}

			} else { //if response channel is closed
				close(typedResponseChan) //close the typed response channel too
				return                   //kill this goroutine
			}
			//the returned error channel is the same channel that is directly contained in the ResponseReader, so we don't need to handle it here
		}
	}()

	return typedResponseChan, rr.errorChan, nil
}

/*
	SubscriptionDataReader
*/

type SubscriptionDataReader struct {
	connectorLogTag             string
	topic                       string
	lastSubscriptionRequestData interface{}
	persistent                  bool
	paused                      bool
	unsubscribing               bool
	dataChan                    chan json.RawMessage
	errorChan                   chan error
	typedDataChanBufferSize     int
	channelsRequested           bool
	channelsClosed              bool
	lock                        sync.Mutex

	dataChanPrevQueue int
	errChanPrevQueue  atomic.Int64
}

func (sdr *SubscriptionDataReader) closeChannels() {
	sdr.lock.Lock()
	defer sdr.lock.Unlock()

	if !sdr.channelsClosed {
		close(sdr.dataChan)
		close(sdr.errorChan)
		sdr.channelsClosed = true
	}
}

func (sdr *SubscriptionDataReader) GetRawSubscriptionDataChannels() (chan json.RawMessage, chan error, error) {
	//use locks to avoid concurrent access to the SubscriptionDataReader (from different goroutines calling GetTypedResponseChannels at the same time)
	sdr.lock.Lock()
	defer sdr.lock.Unlock()

	if sdr.channelsRequested { //if channels for this reader have already been requested
		return nil, nil, DATA_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	sdr.channelsRequested = true

	return sdr.dataChan, sdr.errorChan, nil
}

// GetTypedSubscriptionDataOnChannels does NOT automatically close the channels passed as parameters when the subscription is closed.
// An empty struct is sent on the unsubscribeChan when the subscription is closed. unsubscribeChan can be nil.
func GetTypedSubscriptionDataOnChannels[DataType any](sdr *SubscriptionDataReader, typedDataChan chan *DataType, errorChan chan error, unsubscribeChan chan struct{}) error {
	if typedDataChan == nil {
		return fmt.Errorf("typedDataChan can't be nil")
	}
	if errorChan == nil {
		return fmt.Errorf("errorChan can't be nil")
	}

	//use locks to avoid concurrent access to the SubscriptionDataReader (from different goroutines calling GetTypedResponseChannels at the same time)
	sdr.lock.Lock()
	defer sdr.lock.Unlock()

	if sdr.channelsRequested { //if channels for this reader have already been requested
		return DATA_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	sdr.channelsRequested = true

	//create two goroutines that "translate" all incoming data and errors

	var errChanPrevQueue atomic.Int64

	go func() {
		//avoid crashing the entire process if the specified channels are closed when writing to it
		defer func() {
			if r := recover(); r != nil {
				log.Warningf("[%s][GetTypedSubscriptionDataOnChannels][dataChan goroutine] recovered from panic: %+v\n", sdr.connectorLogTag, r)
			}
		}()

		var rawJsonData json.RawMessage
		var chanOpen bool
		var typedChanPrevQueue int
		for {
			rawJsonData, chanOpen = <-sdr.dataChan
			if chanOpen {
				var obj DataType
				err := jsoniter.ConfigFastest.Unmarshal(rawJsonData, &obj)
				if err == nil { //no error
					typedDataChan <- &obj

					if typedChanPrevQueue != len(typedDataChan) && len(typedDataChan)%10 == 0 {
						log.Warningf("[%s][GetTypedSubscriptionDataOnChannels] typedDataChan queue: %d | topic: %s\n", sdr.connectorLogTag, len(typedDataChan), sdr.topic)
						typedChanPrevQueue = len(typedDataChan)
					}

				} else { //error
					errorChan <- fmt.Errorf("error in jsoniter.Unmarshal(rawJsonData, &obj): %s", err)

					if int(errChanPrevQueue.Load()) != len(errorChan) && len(errorChan)%10 == 0 {
						log.Warningf("[%s][GetTypedSubscriptionDataOnChannels] errorChan queue: %d | topic: %s\n", sdr.connectorLogTag, len(errorChan), sdr.topic)
						errChanPrevQueue.Store(int64(len(errorChan)))
					}
				}

			} else { //if data channel is closed
				return //kill this goroutine
			}
		}
	}()

	go func() {
		//avoid crashing the entire process if the specified channel is closed when writing to it
		defer func() {
			if r := recover(); r != nil {
				log.Warningf("[%s][GetTypedSubscriptionDataOnChannels][errorChan goroutine] recovered from panic: %+v\n", sdr.connectorLogTag, r)
			}
		}()

		var err error
		var chanOpen bool
		for {
			err, chanOpen = <-sdr.errorChan
			if chanOpen {
				errorChan <- err

				if int(errChanPrevQueue.Load()) != len(errorChan) && len(errorChan)%10 == 0 {
					log.Warningf("[%s][GetTypedSubscriptionDataOnChannels] errorChan queue: %d | topic: %s\n", sdr.connectorLogTag, len(errorChan), sdr.topic)
					errChanPrevQueue.Store(int64(len(errorChan)))
				}

			} else { //if error channel is closed
				if unsubscribeChan != nil { //if an unsubscribeChan was specified
					log.Debugf("[%s][GetTypedSubscriptionDataOnChannels] unsubscribeChan is not nil, sending an empty struct on it\n", sdr.connectorLogTag)
					unsubscribeChan <- struct{}{} //notify the unsubscription by sending an empty struct on it
					log.Debugf("[%s][GetTypedSubscriptionDataOnChannels] sent an empty struct on unsubscribeChan\n", sdr.connectorLogTag)
				}
				return //kill this goroutine
			}
		}
	}()

	return nil
}

// GetTypedSubscriptionDataChannels DOES automatically close the returned channels when the subscription is closed
func GetTypedSubscriptionDataChannels[DataType any](sdr *SubscriptionDataReader) (chan *DataType, chan error, error) {
	//use locks to avoid concurrent access to the SubscriptionDataReader (from different goroutines calling GetTypedResponseChannels at the same time)
	sdr.lock.Lock()
	defer sdr.lock.Unlock()

	if sdr.channelsRequested { //if channels for this reader have already been requested
		return nil, nil, DATA_CHANNEL_ALREADY_REQUESTED_ERROR
	}

	sdr.channelsRequested = true

	//create typed data channels
	typedDataChan := make(chan *DataType, sdr.typedDataChanBufferSize)

	//create a goroutine that "translates" all incoming data
	go func() {
		var typedChanPrevQueue int
		for {
			rawJsonData, chanOpen := <-sdr.dataChan
			if chanOpen {
				var obj DataType
				err := jsoniter.ConfigFastest.Unmarshal(rawJsonData, &obj)
				if err == nil { //no error
					typedDataChan <- &obj

					if typedChanPrevQueue != len(typedDataChan) && len(typedDataChan)%10 == 0 {
						log.Warningf("[%s][GetTypedSubscriptionDataChannels] typedDataChan queue: %d | topic: %s\n", sdr.connectorLogTag, len(typedDataChan), sdr.topic)
						typedChanPrevQueue = len(typedDataChan)
					}

				} else { //error
					//we need the lock to correctly check whether the source error channel is closed or not
					//(note that this is a separate goroutine, not the same one that called GetTypedResponseChannels)
					sdr.lock.Lock()
					if !sdr.channelsClosed {
						sdr.errorChan <- fmt.Errorf("error in jsoniter.Unmarshal(rawJsonData, &obj): %s", err)

						if int(sdr.errChanPrevQueue.Load()) != len(sdr.errorChan) && len(sdr.errorChan)%10 == 0 {
							log.Warningf("[%s][GetTypedSubscriptionDataChannels] sdr.errorChan queue: %d\n", sdr.connectorLogTag, len(sdr.errorChan))
							sdr.errChanPrevQueue.Store(int64(len(sdr.errorChan)))
						}
					}
					sdr.lock.Unlock()
				}

			} else { //if data channel is closed
				close(typedDataChan) //close the typed data channel too
				return               //kill this goroutine
			}
			//the returned error channel is the same channel that is directly contained in the SubscriptionDataReader, so we don't need to handle it here
		}
	}()

	return typedDataChan, sdr.errorChan, nil
}
