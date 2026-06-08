package main

import "testing"

func TestBuildDatabaseURLEscapesCredentials(t *testing.T) {
	got := buildDatabaseURL("reply@bot", "pa:ss/word?", "localhost", "5432", "reply db", "disable")
	want := "postgres://reply%40bot:pa%3Ass%2Fword%3F@localhost:5432/reply%20db?sslmode=disable"

	if got != want {
		t.Fatalf("buildDatabaseURL() = %q; want %q", got, want)
	}
}
