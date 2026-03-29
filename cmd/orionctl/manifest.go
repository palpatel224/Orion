package main

import mp "github.com/aditip149209/Orion/cmd/orionctl/internal/manifest"

type submissionKind = mp.Kind

const (
	submissionKindTask submissionKind = mp.KindTask
	submissionKindApp  submissionKind = mp.KindApp
)

func parseSubmissionFile(filePath string) ([]byte, submissionKind, error) {
	return mp.ParseSubmissionFile(filePath)
}

func parseTaskSubmissionFile(filePath string) ([]byte, error) {
	return mp.ParseTaskSubmissionFile(filePath)
}
