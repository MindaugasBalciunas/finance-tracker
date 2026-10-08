package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"ft/internal/ai"
	"ft/internal/notify"
)

// chatJob is a question being answered. It runs on its own context, not the
// request's: a phone that puts the app in the background (and so drops the
// connection) doesn't stop the answer — it lands in the history, and a
// Home Assistant notification says it is ready.
type chatJob struct {
	Question string    `json:"question"`
	Started  time.Time `json:"started"`
	Done     bool      `json:"done"`
	Error    string    `json:"error,omitempty"`

	result   *ai.ChatResult
	err      error
	finished chan struct{}
	left     bool   // the asker disconnected before the answer was ready
	origin   string // where the app is served, for the notification's link
}

type chatJobs struct {
	mu  sync.Mutex
	cur *chatJob
}

// chatBudget bounds one answer, like the assistant's own limit plus room to save.
const chatJobBudget = 5 * time.Minute

var errChatBusy = &HTTPError{http.StatusConflict, "still answering your last question — it will appear here when it's done"}

// start runs ask in the background; one question at a time.
func (s *Server) startChat(question, origin string, ask func(context.Context) (*ai.ChatResult, error)) (*chatJob, error) {
	s.chats.mu.Lock()
	defer s.chats.mu.Unlock()
	if j := s.chats.cur; j != nil && !j.Done {
		return nil, errChatBusy
	}
	j := &chatJob{Question: question, Started: time.Now(), finished: make(chan struct{}), origin: origin}
	s.chats.cur = j
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), chatJobBudget)
		defer cancel()
		res, err := ask(ctx)
		s.chats.mu.Lock()
		j.result, j.err, j.Done = res, err, true
		if err != nil {
			j.Error = err.Error()
		}
		left := j.left
		s.chats.mu.Unlock()
		close(j.finished)
		if left {
			s.notifyAnswer(j)
		}
	}()
	return j, nil
}

// wait returns the answer, or errChatLeft when the asker went away first.
func (s *Server) waitChat(r *http.Request, j *chatJob) (*ai.ChatResult, error) {
	select {
	case <-j.finished:
		return j.result, j.err
	case <-r.Context().Done():
		s.chats.mu.Lock()
		if !j.Done {
			j.left = true
		}
		s.chats.mu.Unlock()
		return nil, errors.New("answer continues in the background")
	}
}

// chatStatus is what the app polls after coming back to the foreground.
func (s *Server) chatStatus() map[string]any {
	s.chats.mu.Lock()
	defer s.chats.mu.Unlock()
	j := s.chats.cur
	if j == nil {
		return map[string]any{"running": false}
	}
	// The app asked again, so it is watching: no notification needed.
	if !j.Done {
		j.left = false
	}
	return map[string]any{"running": !j.Done, "question": j.Question, "started": j.Started, "error": j.Error}
}

// markLeft is called when the app goes to the background mid-answer.
func (s *Server) markChatLeft() {
	s.chats.mu.Lock()
	defer s.chats.mu.Unlock()
	if j := s.chats.cur; j != nil && !j.Done {
		j.left = true
	}
}

func (s *Server) notifyAnswer(j *chatJob) {
	msg := "Your answer is ready — tap to read it."
	if j.err != nil {
		msg = "Ask CFO couldn't finish your question — open the app to see why."
	}
	link := ""
	if j.origin != "" {
		link = strings.TrimRight(j.origin, "/") + "/#/ai"
	}
	notify.Send(s.DB, notify.Message{Title: "Ask CFO", Message: msg, URL: link, Tag: "ask-cfo"})
}
