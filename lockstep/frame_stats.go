package lockstep

import (
	"sync"
	"time"
)

// FrameStats 帧统计信息
type FrameStats struct {
	totalFrames   uint64
	missedFrames  uint64
	lateFrames    uint64
	frameTimeSum  time.Duration
	lastFrameTime time.Time
	mutex         sync.RWMutex
}

// GetTotalFrames 获取总帧数
func (fs *FrameStats) GetTotalFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.totalFrames
}

// GetMissedFrames 获取丢失帧数
func (fs *FrameStats) GetMissedFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.missedFrames
}

// GetLateFrames 获取延迟帧数
func (fs *FrameStats) GetLateFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.lateFrames
}

// GetAverageFrameTime 获取平均帧时间
func (fs *FrameStats) GetAverageFrameTime() time.Duration {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	if fs.totalFrames == 0 {
		return 0
	}
	return fs.frameTimeSum / time.Duration(fs.totalFrames)
}

// UpdateFrameStats 更新帧统计信息
func (fs *FrameStats) UpdateFrameStats(totalFrames, missedFrames, lateFrames uint64, frameTimeSum time.Duration, lastFrameTime time.Time) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	fs.totalFrames = totalFrames
	fs.missedFrames = missedFrames
	fs.lateFrames = lateFrames
	fs.frameTimeSum = frameTimeSum
	fs.lastFrameTime = lastFrameTime
}

// IncrementFrameStats 增量更新帧统计信息
func (fs *FrameStats) IncrementFrameStats(missed, late bool, frameDuration time.Duration) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	fs.totalFrames++
	if missed {
		fs.missedFrames++
	}
	if late {
		fs.lateFrames++
	}
	fs.frameTimeSum += frameDuration
	fs.lastFrameTime = time.Now()
}
