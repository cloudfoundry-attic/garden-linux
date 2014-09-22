// Code generated by protoc-gen-gogo.
// source: stream_in.proto
// DO NOT EDIT!

package garden

import proto "code.google.com/p/gogoprotobuf/proto"
import math "math"

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = math.Inf

type StreamInRequest struct {
	Handle           *string `protobuf:"bytes,1,req,name=handle" json:"handle,omitempty"`
	DstPath          *string `protobuf:"bytes,2,req,name=dst_path" json:"dst_path,omitempty"`
	XXX_unrecognized []byte  `json:"-"`
}

func (m *StreamInRequest) Reset()         { *m = StreamInRequest{} }
func (m *StreamInRequest) String() string { return proto.CompactTextString(m) }
func (*StreamInRequest) ProtoMessage()    {}

func (m *StreamInRequest) GetHandle() string {
	if m != nil && m.Handle != nil {
		return *m.Handle
	}
	return ""
}

func (m *StreamInRequest) GetDstPath() string {
	if m != nil && m.DstPath != nil {
		return *m.DstPath
	}
	return ""
}

type StreamInResponse struct {
	XXX_unrecognized []byte `json:"-"`
}

func (m *StreamInResponse) Reset()         { *m = StreamInResponse{} }
func (m *StreamInResponse) String() string { return proto.CompactTextString(m) }
func (*StreamInResponse) ProtoMessage()    {}

func init() {
}
