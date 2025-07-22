package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

const wsURL = "wss://dashscope.aliyuncs.com/api-ws/v1/inference/" // aliyun WebSocket 服务器地址

var dialer = websocket.DefaultDialer

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Event struct {
	Header  Header  `json:"header"`
	Payload Payload `json:"payload"`
}

type Header struct {
	Event        string `json:"event"`
	ErrorMessage string `json:"error_message"`
	Action       string `json:"action"`
	TaskID       string `json:"task_id"`
	Streaming    string `json:"streaming"`
}

type Payload struct {
	Output     Output `json:"output"`
	TaskGroup  string `json:"task_group"`
	Task       string `json:"task"`
	Function   string `json:"function"`
	Model      string `json:"model"`
	Parameters Params `json:"parameters"`
	Input      Input  `json:"input"`
}

type Params struct {
	Format        string   `json:"format"`
	SampleRate    int      `json:"sample_rate"`
	LanguageHints []string `json:"language_hints"`
}

type Input struct {
}

type Output struct {
	Sentence struct {
		BeginTime int64  `json:"begin_time"`
		EndTime   *int64 `json:"end_time"`
		Text      string `json:"text"`
		Words     []struct {
			BeginTime   int64  `json:"begin_time"`
			EndTime     *int64 `json:"end_time"`
			Text        string `json:"text"`
			Punctuation string `json:"punctuation"`
		} `json:"words"`
	} `json:"sentence"`
}

func main() {
	_ = godotenv.Load()

	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		log.Println("检查环境变量设置")
		return
	}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		aliyunConn, err := connectToAliyun(apiKey)
		if err != nil {
			log.Println("connect aliyun websocket failed, err:", err)
			return
		}
		defer aliyunConn.Close()

		localConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Failed to upgrade connection:", err)
			return
		}
		defer localConn.Close()

		// 启动一个goroutine来接收结果
		taskStarted := make(chan bool)
		taskDone := make(chan bool)
		go forwardAliyunToLocal(aliyunConn, localConn, taskStarted, taskDone)

		// 发送run-task指令
		taskID, err := sendRunTaskCmd(aliyunConn)
		if err != nil {
			log.Println("send run-task cmd failed, err:", err)
			return
		}

		// 等待task-started事件
		waitForTaskStarted(taskStarted)

		for {
			select {
			case <-taskDone:
				return
			default:
				// 读取音频流
				msgType, msg, err := localConn.ReadMessage()
				if err != nil {
					log.Println("Error reading message from local websocket:", err)
					if err := sendFinishTaskCmd(aliyunConn, taskID); err != nil {
						log.Println("send finish-task cmd failed, err:", err)
					}
					return
				}

				// 识别音频流
				if err := aliyunConn.WriteMessage(msgType, msg); err != nil {
					log.Println("Error writing message to aliyun websocket:", err)
					if err := sendFinishTaskCmd(aliyunConn, taskID); err != nil {
						log.Println("send finish-task cmd failed, err:", err)
					}
					return
				}
			}
		}
	})

	log.Println("服务启动于 :8080")
	http.ListenAndServe(":8080", nil)
}

func connectToAliyun(apiKey string) (*websocket.Conn, error) {
	header := http.Header{}
	header.Add("X-DashScope-DataInspection", "enable")
	header.Add("Authorization", "Bearer "+apiKey)
	conn, _, err := dialer.Dial(wsURL, header)
	return conn, err
}

func forwardAliyunToLocal(aliyunConn *websocket.Conn, localConn *websocket.Conn, taskStarted chan<- bool, taskDone chan<- bool) {
	for {
		_, message, err := aliyunConn.ReadMessage()
		if err != nil {
			log.Println("Error reading message:", err)
			return
		}
		var event Event
		if err := json.Unmarshal(message, &event); err != nil {
			log.Println("Error unmarshalling message:", err)
			continue
		}
		if handleEvent(localConn, event, taskStarted, taskDone) {
			return // 结束子协程
		}
	}
}

func handleEvent(localConn *websocket.Conn, event Event, taskStarted chan<- bool, taskDone chan<- bool) bool {
	switch event.Header.Event {
	case "task-started":
		// {
		// 	"header": {
		// 			"task_id": "2bf83b9a-baeb-4fda-8d9a-xxxxxxxxxxxx",
		// 			"event": "task-started",
		// 			"attributes": {}
		// 	},
		// 	"payload": {}
		// }
		log.Println("收到task-started事件")
		taskStarted <- true
	case "task-finished":
		// {
		// 	"header": {
		// 			"task_id": "2bf83b9a-baeb-4fda-8d9a-xxxxxxxxxxxx",
		// 			"event": "task-finished",
		// 			"attributes": {}
		// 	},
		// 	"payload": {
		// 			"output": {},
		// 			"usage": null
		// 	}
		// }
		log.Println("收到task-finished事件")
		taskDone <- true
		return true
	case "task-failed":
		// {
		// 	"header": {
		// 			"task_id": "2bf83b9a-baeb-4fda-8d9a-xxxxxxxxxxxx",
		// 			"event": "task-failed",
		// 			"error_code": "CLIENT_ERROR",
		// 			"error_message": "request timeout after 23 seconds.",
		// 			"attributes": {}
		// 	},
		// 	"payload": {}
		// }
		if event.Header.ErrorMessage != "" {
			log.Println("任务失败:", event.Header.ErrorMessage)
		} else {
			log.Println("未知原因导致任务失败")
		}
		taskDone <- true
		return true
	case "result-generated":
		// {
		// 	"header": {
		// 		"task_id": "2bf83b9a-baeb-4fda-8d9a-xxxxxxxxxxxx",
		// 		"event": "result-generated",
		// 		"attributes": {}
		// 	},
		// 	"payload": {
		// 		"output": {
		// 			"sentence": {
		// 				"begin_time": 170,
		// 				"end_time": null,
		// 				"text": "好，我们的一个",
		// 				"words": [
		// 					{
		// 						"begin_time": 170,
		// 						"end_time": 295,
		// 						"text": "好",
		// 						"punctuation": "，"
		// 					},
		// 					{
		// 						"begin_time": 295,
		// 						"end_time": 503,
		// 						"text": "我们",
		// 						"punctuation": ""
		// 					},
		// 					{
		// 						"begin_time": 503,
		// 						"end_time": 711,
		// 						"text": "的一",
		// 						"punctuation": ""
		// 					},
		// 					{
		// 						"begin_time": 711,
		// 						"end_time": 920,
		// 						"text": "个",
		// 						"punctuation": ""
		// 					}
		// 				]
		// 			}
		// 		},
		// 		"usage": null
		// 	}
		// }

		if event.Payload.Output.Sentence.Text != "" {
			log.Println("识别结果:", event.Payload.Output.Sentence.Text)
			localConn.WriteMessage(websocket.TextMessage, []byte(event.Payload.Output.Sentence.Text))
		}
	default:
		log.Println("Unknown event type:", event)
	}
	return false
}

//	{
//		"header": {
//				"action": "run-task",
//				"task_id": "2bf83b9a-baeb-4fda-8d9a-xxxxxxxxxxxx", // 随机uuid
//				"streaming": "duplex"
//		},
//		"payload": {
//				"task_group": "audio",
//				"task": "asr",
//				"function": "recognition",
//				"model": "paraformer-realtime-v2",
//				"parameters": {
//						"format": "pcm", // 音频格式
//						"sample_rate": 16000, // 采样率
//						"vocabulary_id": "vocab-xxx-24ee19fa8cfb4d52902170a0xxxxxxxx", // paraformer-realtime-v2支持的热词ID
//						"disfluency_removal_enabled": false, // 过滤语气词
//						"language_hints": [
//								"en"
//						] // 指定语言，仅支持paraformer-realtime-v2模型
//				},
//				"resources": [ //不使用热词功能时，不要传递resources参数
//						{
//								"resource_id": "xxxxxxxxxxxx", // paraformer-realtime-v1支持的热词ID
//								"resource_type": "asr_phrase"
//						}
//				],
//				"input": {}
//		}
//	}
//
// 发送run-task指令
func sendRunTaskCmd(aliyunConn *websocket.Conn) (string, error) {
	taskID := uuid.New().String()
	runTaskCmd := Event{
		Header: Header{
			Action:    "run-task",
			TaskID:    taskID,
			Streaming: "duplex",
		},
		Payload: Payload{
			TaskGroup: "audio",
			Task:      "asr",
			Function:  "recognition",
			Model:     "paraformer-realtime-v2",
			Parameters: Params{
				Format:     "pcm", //对于opus和speex格式的音频，需要ogg封装；对于wav格式的音频，需要pcm编码
				SampleRate: 16000,
				LanguageHints: []string{
					"zh", //中文 包括上海话、吴语、闽南语、东北话、甘肃话、贵州话、河南话、湖北话、湖南话、江西话、宁夏话、山西话、陕西话、山东话、四川话、天津话、云南话、粤语
					// "en", //英文
					// "ja",  //日语
					// "yue", //粤语
					// "ko",  //韩语
					// "de",  //德语
					// "fr",  //法语
					// "ru",  //俄语
				},
			},
			Input: Input{},
		},
	}
	err := aliyunConn.WriteJSON(runTaskCmd)
	return taskID, err
}

//	{
//		"header": {
//				"action": "finish-task",
//				"task_id": "2bf83b9a-baeb-4fda-8d9a-xxxxxxxxxxxx",
//				"streaming": "duplex"
//		},
//		"payload": {
//				"input": {}
//		}
//	}
//
// 发送finish-task指令
// 触发task-finished
func sendFinishTaskCmd(aliyunConn *websocket.Conn, taskID string) error {
	finishTaskCmd := Event{
		Header: Header{
			Action:    "finish-task",
			TaskID:    taskID,
			Streaming: "duplex",
		},
		Payload: Payload{
			Input: Input{},
		},
	}
	return aliyunConn.WriteJSON(finishTaskCmd)
}

// 等待task-started事件
func waitForTaskStarted(taskStarted <-chan bool) {
	select {
	case <-taskStarted:
		log.Println("Task started")
	case <-time.After(time.Minute):
		log.Fatal("Timed out waiting for task to start.")
	}
}
