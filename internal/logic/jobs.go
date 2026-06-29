package logic

import (
	"time"

	"portfolio-backend/internal/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type JobEnvelope struct {
	Type     string `json:"type"`
	Payload  any    `json:"payload"`
	QueuedAt string `json:"queuedAt"`
}

type QueuedJob struct {
	Accepted bool        `json:"accepted"`
	Job      JobEnvelope `json:"job"`
}

// QueueContactFollowUp mirrors the in-process stub from the original service.
func QueueContactFollowUp(inquiry *model.ContactInquiry) QueuedJob {
	job := JobEnvelope{
		Type: "contact.follow-up",
		Payload: map[string]any{
			"inquiryId": inquiry.ID,
			"email":     inquiry.Email,
			"subject":   inquiry.Subject,
			"locale":    inquiry.Locale,
		},
		QueuedAt: time.Now().UTC().Format(time.RFC3339),
	}
	logx.Infof("[jobs] queued job %+v", job)
	return QueuedJob{Accepted: true, Job: job}
}
