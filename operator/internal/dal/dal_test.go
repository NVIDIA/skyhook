/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package dal

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"k8s.io/client-go/kubernetes/fake"
)

func TestTailAndSanitize(t *testing.T) {
	// A rune whose UTF-8 encoding is longer than one byte, used to build inputs
	// that a byte-boundary cut would split.
	multibyte := strings.Repeat("é", 100) // 2 bytes each → 200 bytes

	cases := []struct {
		name     string
		input    string
		maxBytes int64
		want     string
		wantErr  bool
	}{
		{name: "shorter than cap returns whole", input: "hello", maxBytes: 1024, want: "hello"},
		{name: "longer than cap keeps the tail", input: "abcdef", maxBytes: 3, want: "def"},
		{name: "zero cap returns empty", input: "abc", maxBytes: 0, want: ""},
		{name: "negative cap returns empty", input: "abc", maxBytes: -1, want: ""},
		{name: "exact cap returns whole", input: "abc", maxBytes: 3, want: "abc"},
		{name: "spans multiple read chunks", input: strings.Repeat("x", 100*1024) + "TAIL", maxBytes: 4, want: "TAIL"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tailAndSanitize(strings.NewReader(tc.input), tc.maxBytes)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	// Invalid bytes — including a multibyte rune the tail cut in half — must come
	// back as valid UTF-8.
	t.Run("output is always valid UTF-8", func(t *testing.T) {
		got, err := tailAndSanitize(strings.NewReader(string([]byte{0xff, 0xfe})+"ok"), 1024)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("result is not valid UTF-8: %q", got)
		}
		if !strings.HasSuffix(got, "ok") {
			t.Fatalf("expected trailing %q in %q", "ok", got)
		}

		// Cut a 2-byte rune in half by capping to an odd tail length.
		cut, err := tailAndSanitize(strings.NewReader(multibyte), 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !utf8.ValidString(cut) {
			t.Fatalf("split-rune result is not valid UTF-8: %q", cut)
		}
	})
}

func TestGetPodLogTail(t *testing.T) {
	t.Run("returns the container logs from the clientset", func(t *testing.T) {
		d := New(nil, fake.NewClientset())
		got, err := d.GetPodLogTail(context.Background(), "skyhook", "pod-1", "step", 1024)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The fake clientset serves a canned "fake logs" body for GetLogs.
		if !strings.Contains(got, "fake logs") {
			t.Fatalf("got %q, want it to contain %q", got, "fake logs")
		}
	})

	t.Run("errors when no clientset is configured", func(t *testing.T) {
		d := New(nil, nil)
		if _, err := d.GetPodLogTail(context.Background(), "skyhook", "pod-1", "step", 1024); err == nil {
			t.Fatal("expected an error when clientset is nil, got nil")
		}
	})
}
