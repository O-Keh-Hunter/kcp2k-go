package lockstep

import (
	"testing"
)

func TestInputBufferMultipleInputs(t *testing.T) {
	// 测试新的 InputBuffer 数据结构
	player := &Player{
		ID:          1,
		InputBuffer: make(map[FrameID][]*InputMessage),
	}

	// 创建测试输入消息
	input1 := &InputMessage{
		SequenceId: 100,
		Timestamp:  1234567890,
		Data:       []byte("test input 1"),
	}

	input2 := &InputMessage{
		SequenceId: 101,
		Timestamp:  1234567891,
		Data:       []byte("test input 2"),
	}

	// 测试添加多个输入到同一帧
	frameID := FrameID(10)
	
	// 添加第一个输入
	if existingInputs, exists := player.InputBuffer[frameID]; exists {
		player.InputBuffer[frameID] = append(existingInputs, input1)
	} else {
		player.InputBuffer[frameID] = []*InputMessage{input1}
	}

	// 添加第二个输入
	if existingInputs, exists := player.InputBuffer[frameID]; exists {
		player.InputBuffer[frameID] = append(existingInputs, input2)
	} else {
		player.InputBuffer[frameID] = []*InputMessage{input2}
	}

	// 验证结果
	if inputList, exists := player.InputBuffer[frameID]; exists {
		if len(inputList) != 2 {
			t.Errorf("Expected 2 inputs, got %d", len(inputList))
		}
		
		if inputList[0].SequenceId != 100 {
			t.Errorf("Expected first input sequence ID 100, got %d", inputList[0].SequenceId)
		}
		
		if inputList[1].SequenceId != 101 {
			t.Errorf("Expected second input sequence ID 101, got %d", inputList[1].SequenceId)
		}
		
		if string(inputList[0].Data) != "test input 1" {
			t.Errorf("Expected first input data 'test input 1', got '%s'", string(inputList[0].Data))
		}
		
		if string(inputList[1].Data) != "test input 2" {
			t.Errorf("Expected second input data 'test input 2', got '%s'", string(inputList[1].Data))
		}
	} else {
		t.Errorf("No inputs found for frame %d", frameID)
	}

	t.Logf("InputBuffer modification test completed successfully!")
}

func TestInputBufferEmptyFrame(t *testing.T) {
	// 测试空帧的处理
	player := &Player{
		ID:          1,
		InputBuffer: make(map[FrameID][]*InputMessage),
	}

	frameID := FrameID(20)
	
	// 检查不存在的帧
	if inputList, exists := player.InputBuffer[frameID]; exists {
		if len(inputList) > 0 {
			t.Errorf("Expected empty frame, but got %d inputs", len(inputList))
		}
	} else {
		// 这是期望的行为
		t.Logf("Frame %d correctly has no inputs", frameID)
	}
}