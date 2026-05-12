package main

import (
"reflect"
"testing"
)

func TestNormalizeProfileArgs(t *testing.T) {
tests := []struct {
name string
in   []string
want []string
}{
{name: "no profile", in: []string{"search", "abc"}, want: []string{"search", "abc"}},
{name: "short profile after cmd", in: []string{"search", "-p", "work", "abc"}, want: []string{"--profile", "work", "search", "abc"}},
{name: "long profile equals", in: []string{"search", "--profile=work", "abc"}, want: []string{"--profile", "work", "search", "abc"}},
}
for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
got := normalizeProfileArgs(tt.in)
if !reflect.DeepEqual(got, tt.want) {
t.Fatalf("got %v, want %v", got, tt.want)
}
})
}
}

func TestNormalizeNotionID(t *testing.T) {
got, err := normalizeNotionID("3c90c3cc0d444b5088888dd25736052a", "page")
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
want := "3c90c3cc-0d44-4b50-8888-8dd25736052a"
if got != want {
t.Fatalf("got %s, want %s", got, want)
}
}

func TestParseSlice(t *testing.T) {
got, err := parseSlice("2-5")
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if got != [2]int{2, 5} {
t.Fatalf("got %v, want [2 5]", got)
}
}
