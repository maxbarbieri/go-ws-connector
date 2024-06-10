package test

import (
	wsconnector "github.com/maxbarbieri/go-ws-connector"
	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"strconv"
	"testing"
)

type EchoMsg struct {
	Msg0     string            `json:"msg0"`
	Msg1     string            `json:"msg1"`
	Msg2     string            `json:"msg2"`
	Msg3     string            `json:"msg3"`
	Msg4     string            `json:"msg4"`
	Msg5     string            `json:"msg5"`
	InnerMsg map[string]string `json:"inner_msg"`
}

func generateEchoMessage() EchoMsg {
	innerMap := make(map[string]string)
	for i := 0; i < 5; i++ {
		innerMap[strconv.Itoa(i)] = "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234"
		for j := 0; j < i; j++ {
			innerMap[strconv.Itoa(i)] += "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234"
		}
	}
	return EchoMsg{
		Msg0:     "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234",
		Msg1:     "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234",
		Msg2:     "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234",
		Msg3:     "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234",
		Msg4:     "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234",
		Msg5:     "abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234_abcd1234",
		InnerMsg: innerMap,
	}
}

func Benchmark_WsConnector_Echo_Json(b *testing.B) {
	wsc, _ := wsconnector.NewClientConnectorWithDefaultParameters("ws://localhost:3000/wsc", nil, nil, "wsc", nil, nil, nil)
	var resReader *wsconnector.ResponseReader
	echoMsg := generateEchoMessage()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resReader, _ = wsc.SendRequest("echo", echoMsg, true)
		resChan := make(chan *EchoMsg, 1)
		errChan := make(chan error, 1)
		_ = wsconnector.GetTypedResponseOnChannels[EchoMsg](resReader, resChan, errChan)
		<-resChan
	}
}

func Benchmark_WsConnection_Echo_Json(b *testing.B) {
	wsConn, _, _ := websocket.DefaultDialer.Dial("ws://localhost:3000/ws_json", nil)
	echoMsg := generateEchoMessage()
	var writeMsgBytes []byte
	var readMsgBytes []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeMsgBytes, _ = jsoniter.ConfigFastest.Marshal(&echoMsg)
		_ = wsConn.WriteMessage(websocket.TextMessage, writeMsgBytes)
		_, readMsgBytes, _ = wsConn.ReadMessage()
		var readMsg EchoMsg
		_ = jsoniter.ConfigFastest.Unmarshal(readMsgBytes, &readMsg)
	}
}

func Benchmark_WsConnection_Echo_Raw(b *testing.B) {
	wsConn, _, _ := websocket.DefaultDialer.Dial("ws://localhost:3000/ws_raw", nil)
	echoMsg := generateEchoMessage()
	var msgBytes []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgBytes, _ = jsoniter.ConfigFastest.Marshal(&echoMsg)
		_ = wsConn.WriteMessage(websocket.TextMessage, msgBytes)
		_, msgBytes, _ = wsConn.ReadMessage()
	}
}
