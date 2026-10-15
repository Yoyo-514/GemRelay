package gateway

// /v1/videos —— OpenAI（Sora）形状的异步视频生成端点。
//
// gemini 视频要几十秒到几分钟，同步阻塞一个 HTTP 请求不现实，所以照 OpenAI Sora 那套走异步：
//   POST /v1/videos               建任务，立即返回 {id, status:"queued"}
//   GET  /v1/videos/{id}          轮询状态 queued|in_progress|completed|failed
//   GET  /v1/videos/{id}/content  完成后下 MP4
// 底层复用 callGemini（跟 /v1/chat/completions 里 model=gemini-video 同一条链），
// 只是把「阻塞等结果」挪到后台 goroutine，前台立即返 id。

import (
	"sync"
	"time"
)

type VideoJob struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Model     string `json:"model"`
	Status    string `json:"status"` // queued | in_progress | completed | failed
	CreatedAt int64  `json:"created_at"`
	Prompt    string `json:"prompt,omitempty"`
	Error     string `json:"error,omitempty"`

	mp4  []byte // 完成后的视频字节，不进 JSON
	mime string
}

var (
	videoJobs   = map[string]*VideoJob{}
	videoJobsMu sync.Mutex
)

func putVideoJob(j *VideoJob) {
	videoJobsMu.Lock()
	// 顺手清掉 2 小时前的旧任务，别让内存里越堆越多。
	cutoff := time.Now().Unix() - 2*3600
	for id, old := range videoJobs {
		if old.CreatedAt < cutoff {
			delete(videoJobs, id)
		}
	}
	videoJobs[j.ID] = j
	videoJobsMu.Unlock()
}

func VideoJobByID(id string) *VideoJob {
	videoJobsMu.Lock()
	defer videoJobsMu.Unlock()
	j := videoJobs[id]
	if j == nil {
		return nil
	}
	snapshot := *j
	return &snapshot
}

// StartVideo 创建后台任务并返回初始状态快照。
func StartVideo(prompt, model string) *VideoJob {
	j := &VideoJob{ID: "video_" + RandHex(16), Object: "video", Model: model, Status: "queued", CreatedAt: time.Now().Unix(), Prompt: prompt}
	putVideoJob(j)
	snapshot := *j
	go runVideoJob(j)
	return &snapshot
}

// Content 返回状态快照中的视频产物，未完成时字节为空。
func (j *VideoJob) Content() ([]byte, string) { return j.mp4, j.mime }

func runVideoJob(j *VideoJob) {
	videoJobsMu.Lock()
	j.Status = "in_progress"
	videoJobsMu.Unlock()

	_, mc, err := ResolveModel(j.Model)
	if err != nil {
		finishVideoJob(j, nil, "", err.Error())
		return
	}
	_, _, res, err := CallGemini(j.Prompt, j.Prompt, mc, nil, nil, nil, nil)
	if err != nil {
		RecordRequest("videos", j.Model, j.Prompt, "", res, 502, err.Error(), false)
		finishVideoJob(j, nil, "", err.Error())
		return
	}
	if res == nil || len(res.Artifacts) == 0 {
		RecordRequest("videos", j.Model, j.Prompt, "", res, 502, "no video produced", false)
		finishVideoJob(j, nil, "", "no video produced")
		return
	}
	a := res.Artifacts[0]
	RecordRequest("videos", j.Model, j.Prompt, "", res, 200, "", false)
	finishVideoJob(j, a.Data, a.Mime, "")
}

func finishVideoJob(j *VideoJob, mp4 []byte, mime, errStr string) {
	videoJobsMu.Lock()
	defer videoJobsMu.Unlock()
	if errStr != "" {
		j.Status = "failed"
		j.Error = errStr
		return
	}
	j.mp4 = mp4
	j.mime = mime
	if j.mime == "" {
		j.mime = "video/mp4"
	}
	j.Status = "completed"
}
