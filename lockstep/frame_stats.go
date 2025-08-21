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
	emptyFrames   uint64 // 没有Input的帧数
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

// GetFrameTimeSum 获取帧时间总和
func (fs *FrameStats) GetFrameTimeSum() time.Duration {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.frameTimeSum
}

// GetEmptyFrames 获取空帧数（没有Input的帧）
func (fs *FrameStats) GetEmptyFrames() uint64 {
	fs.mutex.RLock()
	defer fs.mutex.RUnlock()
	return fs.emptyFrames
}

// IncrementFrameStats 增量更新帧统计信息
func (fs *FrameStats) IncrementFrameStats(missed, late, empty bool, frameDuration time.Duration) {
	fs.mutex.Lock()
	defer fs.mutex.Unlock()
	fs.totalFrames++
	if missed {
		fs.missedFrames++
	}
	if late {
		fs.lateFrames++
	}
	if empty {
		fs.emptyFrames++
	}
	fs.frameTimeSum += frameDuration
	fs.lastFrameTime = time.Now()
}
