package proto_test

import (
	"testing"

	pb "github.com/PhilipSchmid/echo-observatory/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestV2DescriptorPreservesWireIdentity(t *testing.T) {
	options, ok := pb.File_proto_echo_proto.Options().(*descriptorpb.FileOptions)
	require.True(t, ok)
	assert.Equal(t, "github.com/PhilipSchmid/echo-observatory/v2/proto", options.GetGoPackage())
	assert.Equal(t, "echo.EchoService", string(pb.File_proto_echo_proto.Services().Get(0).FullName()))
	assert.Equal(t, "/echo.EchoService/Echo", pb.EchoService_Echo_FullMethodName)
	fields := pb.File_proto_echo_proto.Messages().ByName("EchoResponse").Fields()
	assert.EqualValues(t, 6, fields.ByName("source_ip").Number())
	assert.EqualValues(t, 7, fields.ByName("grpc_method").Number())
	assert.EqualValues(t, 8, fields.ByName("ip_version").Number())
}
