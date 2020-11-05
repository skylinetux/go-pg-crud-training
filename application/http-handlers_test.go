package main

import (
	"net/http"
	"testing"
)

func Test_handleSaveBook(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handleSaveBook(tt.args.w, tt.args.r)
		})
	}
}

func Test_handleListBooks(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handleListBooks(tt.args.w, tt.args.r)
		})
	}
}

func Test_handleViewBook(t *testing.T) {
	type args struct {
		w http.ResponseWriter
		r *http.Request
	}
	tests := []struct {
		name string
		args args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handleViewBook(tt.args.w, tt.args.r)
		})
	}
}

//func TestAbc(t *testing.T) {
//	t.Error() // to indicate test failed
//}

