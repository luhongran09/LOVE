package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func (a *App) storyRoute(api *gin.RouterGroup) {
	api.POST("/story", func(c *gin.Context) {
		memories, err := a.memories(current(c).SpaceID)
		if err != nil {
			fail(c, 500, "加载回忆失败")
			return
		}
		if len(memories) == 0 {
			fail(c, 400, "先添加一段回忆吧")
			return
		}
		if len(memories) > 20 {
			memories = memories[:20]
		}
		lines := []string{}
		for i := len(memories) - 1; i >= 0; i-- {
			m := memories[i]
			description := []rune(m.Description)
			if len(description) > 500 {
				description = description[:500]
			}
			lines = append(lines, fmt.Sprintf("%s，在%s，%s。%s", m.Date, m.Place, m.Title, string(description)))
		}
		key := os.Getenv("LLM_API_KEY")
		if key == "" {
			c.JSON(200, gin.H{"source": "local", "story": "《一起走过的日子》\n\n" + strings.Join(lines, "\n\n") + "\n\n这些看似普通的片刻，被我们留在了同一本相册里。下一段故事，继续一起写。"})
			return
		}
		base := os.Getenv("LLM_BASE_URL")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		parsed, err := url.Parse(base)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			fail(c, 500, "模型服务地址配置无效")
			return
		}
		model := os.Getenv("LLM_MODEL")
		if model == "" {
			fail(c, 500, "请先配置 LLM_MODEL")
			return
		}
		body, _ := json.Marshal(gin.H{"model": model, "messages": []gin.H{{"role": "system", "content": "请仅根据用户提供的真实回忆，用中文写一篇温暖克制的双人旅行日记。不要编造地点、对话、经历或人物信息。"}, {"role": "user", "content": strings.Join(lines, "\n")}}, "max_tokens": 1500})
		req, err := http.NewRequestWithContext(c.Request.Context(), "POST", strings.TrimRight(base, "/")+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			fail(c, 500, "模型请求失败")
			return
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		response, err := client.Do(req)
		if err != nil {
			fail(c, 502, "模型服务暂时不可用")
			return
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			fail(c, 502, "模型服务返回错误，请检查配置和额度")
			return
		}
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result) != nil || len(result.Choices) == 0 {
			fail(c, 502, "模型响应格式无效")
			return
		}
		c.JSON(200, gin.H{"source": "model", "story": result.Choices[0].Message.Content})
	})
}
