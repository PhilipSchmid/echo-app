// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v5.29.3
// source: echo.proto

package echo

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type EchoRequest struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *EchoRequest) Reset() {
	*x = EchoRequest{}
	mi := &file_echo_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EchoRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EchoRequest) ProtoMessage() {}

func (x *EchoRequest) ProtoReflect() protoreflect.Message {
	mi := &file_echo_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EchoRequest.ProtoReflect.Descriptor instead.
func (*EchoRequest) Descriptor() ([]byte, []int) {
	return file_echo_proto_rawDescGZIP(), []int{0}
}

type EchoResponse struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Timestamp     string                 `protobuf:"bytes,1,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	Message       string                 `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"`
	SourceIp      string                 `protobuf:"bytes,3,opt,name=source_ip,json=sourceIp,proto3" json:"source_ip,omitempty"`
	Hostname      string                 `protobuf:"bytes,4,opt,name=hostname,proto3" json:"hostname,omitempty"`
	Listener      string                 `protobuf:"bytes,5,opt,name=listener,proto3" json:"listener,omitempty"`
	Node          string                 `protobuf:"bytes,6,opt,name=node,proto3" json:"node,omitempty"`
	GrpcMethod    string                 `protobuf:"bytes,7,opt,name=grpc_method,json=grpcMethod,proto3" json:"grpc_method,omitempty"`
	HttpVersion   string                 `protobuf:"bytes,8,opt,name=http_version,json=httpVersion,proto3" json:"http_version,omitempty"`
	HttpMethod    string                 `protobuf:"bytes,9,opt,name=http_method,json=httpMethod,proto3" json:"http_method,omitempty"`
	HttpEndpoint  string                 `protobuf:"bytes,10,opt,name=http_endpoint,json=httpEndpoint,proto3" json:"http_endpoint,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *EchoResponse) Reset() {
	*x = EchoResponse{}
	mi := &file_echo_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EchoResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EchoResponse) ProtoMessage() {}

func (x *EchoResponse) ProtoReflect() protoreflect.Message {
	mi := &file_echo_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EchoResponse.ProtoReflect.Descriptor instead.
func (*EchoResponse) Descriptor() ([]byte, []int) {
	return file_echo_proto_rawDescGZIP(), []int{1}
}

func (x *EchoResponse) GetTimestamp() string {
	if x != nil {
		return x.Timestamp
	}
	return ""
}

func (x *EchoResponse) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *EchoResponse) GetSourceIp() string {
	if x != nil {
		return x.SourceIp
	}
	return ""
}

func (x *EchoResponse) GetHostname() string {
	if x != nil {
		return x.Hostname
	}
	return ""
}

func (x *EchoResponse) GetListener() string {
	if x != nil {
		return x.Listener
	}
	return ""
}

func (x *EchoResponse) GetNode() string {
	if x != nil {
		return x.Node
	}
	return ""
}

func (x *EchoResponse) GetGrpcMethod() string {
	if x != nil {
		return x.GrpcMethod
	}
	return ""
}

func (x *EchoResponse) GetHttpVersion() string {
	if x != nil {
		return x.HttpVersion
	}
	return ""
}

func (x *EchoResponse) GetHttpMethod() string {
	if x != nil {
		return x.HttpMethod
	}
	return ""
}

func (x *EchoResponse) GetHttpEndpoint() string {
	if x != nil {
		return x.HttpEndpoint
	}
	return ""
}

var File_echo_proto protoreflect.FileDescriptor

var file_echo_proto_rawDesc = string([]byte{
	0x0a, 0x0a, 0x65, 0x63, 0x68, 0x6f, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x04, 0x65, 0x63,
	0x68, 0x6f, 0x22, 0x0d, 0x0a, 0x0b, 0x45, 0x63, 0x68, 0x6f, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x22, 0xb9, 0x02, 0x0a, 0x0c, 0x45, 0x63, 0x68, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x1b, 0x0a, 0x09, 0x73, 0x6f,
	0x75, 0x72, 0x63, 0x65, 0x5f, 0x69, 0x70, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x49, 0x70, 0x12, 0x1a, 0x0a, 0x08, 0x68, 0x6f, 0x73, 0x74, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x68, 0x6f, 0x73, 0x74, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x6c, 0x69, 0x73, 0x74, 0x65, 0x6e, 0x65, 0x72, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6c, 0x69, 0x73, 0x74, 0x65, 0x6e, 0x65, 0x72, 0x12,
	0x12, 0x0a, 0x04, 0x6e, 0x6f, 0x64, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e,
	0x6f, 0x64, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x67, 0x72, 0x70, 0x63, 0x5f, 0x6d, 0x65, 0x74, 0x68,
	0x6f, 0x64, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x67, 0x72, 0x70, 0x63, 0x4d, 0x65,
	0x74, 0x68, 0x6f, 0x64, 0x12, 0x21, 0x0a, 0x0c, 0x68, 0x74, 0x74, 0x70, 0x5f, 0x76, 0x65, 0x72,
	0x73, 0x69, 0x6f, 0x6e, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x68, 0x74, 0x74, 0x70,
	0x56, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x1f, 0x0a, 0x0b, 0x68, 0x74, 0x74, 0x70, 0x5f,
	0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x68, 0x74,
	0x74, 0x70, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x12, 0x23, 0x0a, 0x0d, 0x68, 0x74, 0x74, 0x70,
	0x5f, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0c, 0x68, 0x74, 0x74, 0x70, 0x45, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x32, 0x3c, 0x0a,
	0x0b, 0x45, 0x63, 0x68, 0x6f, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x2d, 0x0a, 0x04,
	0x45, 0x63, 0x68, 0x6f, 0x12, 0x11, 0x2e, 0x65, 0x63, 0x68, 0x6f, 0x2e, 0x45, 0x63, 0x68, 0x6f,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x12, 0x2e, 0x65, 0x63, 0x68, 0x6f, 0x2e, 0x45,
	0x63, 0x68, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42, 0x0e, 0x5a, 0x0c, 0x2e,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x3b, 0x65, 0x63, 0x68, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
})

var (
	file_echo_proto_rawDescOnce sync.Once
	file_echo_proto_rawDescData []byte
)

func file_echo_proto_rawDescGZIP() []byte {
	file_echo_proto_rawDescOnce.Do(func() {
		file_echo_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_echo_proto_rawDesc), len(file_echo_proto_rawDesc)))
	})
	return file_echo_proto_rawDescData
}

var file_echo_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_echo_proto_goTypes = []any{
	(*EchoRequest)(nil),  // 0: echo.EchoRequest
	(*EchoResponse)(nil), // 1: echo.EchoResponse
}
var file_echo_proto_depIdxs = []int32{
	0, // 0: echo.EchoService.Echo:input_type -> echo.EchoRequest
	1, // 1: echo.EchoService.Echo:output_type -> echo.EchoResponse
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_echo_proto_init() }
func file_echo_proto_init() {
	if File_echo_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_echo_proto_rawDesc), len(file_echo_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_echo_proto_goTypes,
		DependencyIndexes: file_echo_proto_depIdxs,
		MessageInfos:      file_echo_proto_msgTypes,
	}.Build()
	File_echo_proto = out.File
	file_echo_proto_goTypes = nil
	file_echo_proto_depIdxs = nil
}
