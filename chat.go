package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	Url          = "https://api.openai.com/v1/chat/completions"
	Model        = "gpt-3.5-turbo"
	SystemPrompt = "You are a helpful assistant"
	Timeout      = 30 * time.Second
	RecentMsgNum = 10
)
const (
	UserLabel      = "User: "
	AssistantLabel = "Assistant: "
)
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

var token string

type ChatBodyMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatBody struct {
	Model    string        `json:"model"`
	Messages []ChatBodyMsg `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ChatStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		Index        int `json:"index"`
		FinishReason any `json:"finish_reason"`
	} `json:"choices"`
}

func main() {
	token = os.Getenv("OPENAI_API_KEY")
	if token == "" {
		panic("OPENAI_API_KEY is not set in your environment")
	}

	systemMsg := ChatBodyMsg{
		Role:    RoleSystem,
		Content: SystemPrompt,
	}
	parentMsgs := []ChatBodyMsg{}
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print(UserLabel)

		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			continue
		}
		if input == "q" ||
			input == "quit" ||
			input == "exit" {
			break
		}

		msgStart := math.Max(float64(len(parentMsgs)-RecentMsgNum*2), 0)
		buf, err := chatCompletion(
			input,
			append([]ChatBodyMsg{systemMsg}, parentMsgs[int(msgStart):]...)...,
		)
		if err != nil {
			panic(err)
		}
		fmt.Print("\n\n")

		parentMsgs = append(
			parentMsgs,
			ChatBodyMsg{RoleUser, input},
			ChatBodyMsg{RoleAssistant, string(buf)},
		)
	}
}

func chatCompletion(input string, parentMsgs ...ChatBodyMsg) ([]byte, error) {
	msg := ChatBodyMsg{
		Role:    RoleUser,
		Content: input,
	}

	body := &ChatBody{
		Model:    Model,
		Messages: append(parentMsgs, msg),
		Stream:   true,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", Url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: Timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bad response: " + string(body))
	}

	answer := bytes.NewBuffer(nil)
	fmt.Print(AssistantLabel)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}

		line = bytes.TrimSpace(line)

		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		line = bytes.TrimPrefix(line, []byte("data: "))

		if bytes.HasPrefix(line, []byte("[DONE]")) {
			break
		}

		ret := new(ChatStreamResponse)
		if err := json.Unmarshal(line, ret); err != nil {
			return nil, fmt.Errorf("invalid json stream data: %v", err)
		}

		content := ret.Choices[0].Delta.Content
		answer.WriteString(content)
		fmt.Print(content)
	}
	return answer.Bytes(), nil
}
