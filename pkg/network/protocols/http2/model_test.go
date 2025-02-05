// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2/hpack"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

func TestHTTP2LongPath(t *testing.T) {
	tests := []struct {
		name           string
		rawPath        string
		expectedPath   string
		huffmanEnabled bool
		outBufSize     int
	}{
		{
			name:           "Long path with huffman with bigger out buffer",
			rawPath:        fmt.Sprintf("/%s", strings.Repeat("a", maxHTTP2Path+1)),
			huffmanEnabled: true,
		},
		{
			name:           "Long path with huffman with shorter out buffer",
			rawPath:        fmt.Sprintf("/%s", strings.Repeat("a", maxHTTP2Path+1)),
			expectedPath:   fmt.Sprintf("/%s", strings.Repeat("a", 19)),
			huffmanEnabled: true,
			outBufSize:     20,
		},
		{
			name:    "Long path without huffman with bigger out buffer",
			rawPath: fmt.Sprintf("/%s", strings.Repeat("a", maxHTTP2Path+1)),
			// The path is truncated to maxHTTP2Path (including the leading '/')
			expectedPath: fmt.Sprintf("/%s", strings.Repeat("a", maxHTTP2Path-1)),
		},
		{
			name:         "Long path without huffman with shorter out buffer",
			rawPath:      fmt.Sprintf("/%s", strings.Repeat("a", maxHTTP2Path+1)),
			expectedPath: fmt.Sprintf("/%s", strings.Repeat("a", 19)),
			outBufSize:   20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf []byte
			var arr [maxHTTP2Path]uint8
			if tt.huffmanEnabled {
				buf = hpack.AppendHuffmanString(buf, tt.rawPath)
			} else {
				buf = append(buf, tt.rawPath...)
			}
			copy(arr[:], buf)

			request := &EbpfTx{
				Stream: HTTP2Stream{
					Path: http2Path{
						Is_huffman_encoded: tt.huffmanEnabled,
						Raw_buffer:         arr,
						Length:             uint8(len(buf)),
					},
				},
			}

			if tt.outBufSize == 0 {
				tt.outBufSize = http.BufferSize
			}
			outBuf := make([]byte, tt.outBufSize)

			path, ok := request.Path(outBuf)
			require.True(t, ok)
			expectedPath := tt.rawPath
			if tt.expectedPath != "" {
				expectedPath = tt.expectedPath
			}
			assert.Equal(t, expectedPath, string(path))
		})
	}
}

func TestHTTP2Path(t *testing.T) {
	tests := []struct {
		name         string
		rawPath      string
		expectedPath string
		expectedErr  bool
	}{
		{
			name:    "Sanity",
			rawPath: "/hello.HelloService/SayHello",
		},
		{
			name:        "Path does not start with /",
			rawPath:     "hello.HelloService/SayHello",
			expectedErr: true,
		},
		{
			name:        "Empty path",
			rawPath:     "",
			expectedErr: true,
		},
		{
			name:         "Query string",
			rawPath:      "/foo/bar?a=1&b=2",
			expectedPath: "/foo/bar",
		},
	}

	for _, tt := range tests {
		for _, huffmanEnabled := range []bool{false, true} {
			testNameSuffix := fmt.Sprintf("huffman-enabled=%v", huffmanEnabled)
			t.Run(tt.name+testNameSuffix, func(t *testing.T) {
				var buf []byte
				var arr [maxHTTP2Path]uint8
				if huffmanEnabled {
					buf = hpack.AppendHuffmanString(buf, tt.rawPath)
				} else {
					buf = append(buf, tt.rawPath...)
				}
				copy(arr[:], buf)

				request := &EbpfTx{
					Stream: HTTP2Stream{
						Path: http2Path{
							Is_huffman_encoded: huffmanEnabled,
							Raw_buffer:         arr,
							Length:             uint8(len(buf)),
						},
					},
				}

				outBuf := make([]byte, 200)

				path, ok := request.Path(outBuf)
				if tt.expectedErr {
					assert.False(t, ok)
					return
				}
				assert.True(t, ok)
				expectedPath := tt.rawPath
				if tt.expectedPath != "" {
					expectedPath = tt.expectedPath
				}
				assert.Equal(t, expectedPath, string(path))
			})
		}
	}
}

func TestHTTP2Method(t *testing.T) {
	tests := []struct {
		name   string
		Stream HTTP2Stream
		want   http.Method
	}{
		{
			name: "Sanity method test",
			Stream: HTTP2Stream{
				Request_method: http2requestMethod{
					Raw_buffer:         [7]uint8{0x50, 0x55, 0x54},
					Is_huffman_encoded: false,
					Static_table_entry: 0,
					Length:             3,
					Finalized:          false,
				},
			},
			want: http.MethodPut,
		},
		{
			name: "Test method length is bigger than raw buffer size",
			Stream: HTTP2Stream{
				Request_method: http2requestMethod{
					Raw_buffer:         [7]uint8{1, 2},
					Is_huffman_encoded: false,
					Static_table_entry: 0,
					Length:             8,
					Finalized:          false,
				},
			},
			want: http.MethodUnknown,
		},
		{
			name: "Test method length is zero",
			Stream: HTTP2Stream{
				Request_method: http2requestMethod{
					Raw_buffer:         [7]uint8{1, 2},
					Is_huffman_encoded: true,
					Static_table_entry: 0,
					Length:             0,
					Finalized:          false,
				},
			},
			want: http.MethodUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &EbpfTx{
				Stream: tt.Stream,
			}
			assert.Equalf(t, tt.want, tx.Method(), "Method()")
		})
	}
}
