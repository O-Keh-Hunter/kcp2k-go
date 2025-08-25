package lockstep

import (
	"context"
	"fmt"
	"net"

	pb "github.com/O-Keh-Hunter/kcp2k-go/lockstep/proto"
	"google.golang.org/grpc"
)

// RoomServiceServer gRPC 房间服务实现
type RoomServiceServer struct {
	pb.UnimplementedRoomServiceServer
	lockstepServer *LockStepServer
}

// NewRoomServiceServer 创建房间服务服务器
func NewRoomServiceServer(lockstepServer *LockStepServer) *RoomServiceServer {
	return &RoomServiceServer{
		lockstepServer: lockstepServer,
	}
}

// CreateRoom 实现创建房间接口
func (s *RoomServiceServer) CreateRoom(ctx context.Context, req *pb.CreateRoomRequest) (*pb.CreateRoomResponse, error) {
	if req.RoomConfig == nil {
		return &pb.CreateRoomResponse{
			Base: &pb.BaseResponse{
				ErrorCode:    pb.ErrorCode_ERROR_CODE_UNKNOWN,
				ErrorMessage: "room config cannot be nil",
			},
		}, nil
	}
	if req.RoomConfig.MaxPlayers <= 0 {
		return &pb.CreateRoomResponse{
			Base: &pb.BaseResponse{
				ErrorCode:    pb.ErrorCode_ERROR_CODE_UNKNOWN,
				ErrorMessage: "max players must be greater than 0",
			},
		}, nil
	}
	if req.RoomConfig.FrameRate <= 0 {
		return &pb.CreateRoomResponse{
			Base: &pb.BaseResponse{
				ErrorCode:    pb.ErrorCode_ERROR_CODE_UNKNOWN,
				ErrorMessage: "frame rate must be greater than 0",
			},
		}, nil
	}

	// 创建房间配置
	roomConfig := &RoomConfig{
		MaxPlayers:  req.RoomConfig.MaxPlayers,
		FrameRate:   req.RoomConfig.FrameRate,
		MinPlayers:  req.RoomConfig.MinPlayers,
		RetryWindow: req.RoomConfig.RetryWindow,
	}

	// 转换玩家ID列表
	var playerIDs []PlayerID
	for _, pid := range req.PlayerIds {
		playerIDs = append(playerIDs, PlayerID(pid))
	}

	// 调用 LockStepServer 的 CreateRoom 方法
	room, err := s.lockstepServer.CreateRoom(roomConfig, playerIDs)
	if err != nil {
		return &pb.CreateRoomResponse{
			Base: &pb.BaseResponse{
				ErrorCode:    pb.ErrorCode_ERROR_CODE_UNKNOWN,
				ErrorMessage: fmt.Sprintf("failed to create room: %v", err),
			},
		}, nil
	}

	return &pb.CreateRoomResponse{
		Base: &pb.BaseResponse{
			ErrorCode:    pb.ErrorCode_ERROR_CODE_SUCC,
			ErrorMessage: "Room created successfully",
		},
		RoomId: string(room.ID),
	}, nil
}

// GrpcServer gRPC 服务器
type GrpcServer struct {
	server         *grpc.Server
	lockstepServer *LockStepServer
	port           int
}

// NewGrpcServer 创建 gRPC 服务器
func NewGrpcServer(lockstepServer *LockStepServer, port int) *GrpcServer {
	grpcServer := grpc.NewServer()

	roomService := NewRoomServiceServer(lockstepServer)
	pb.RegisterRoomServiceServer(grpcServer, roomService)

	return &GrpcServer{
		server:         grpcServer,
		lockstepServer: lockstepServer,
		port:           port,
	}
}

// Start 启动 gRPC 服务器
func (s *GrpcServer) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	Log.Info("[gRPC] Server starting on port %d", s.port)

	if err := s.server.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

// Stop 停止 gRPC 服务器
func (s *GrpcServer) Stop() {
	if s.server != nil {
		s.server.GracefulStop()
		Log.Info("[gRPC] Server stopped")
	}
}
