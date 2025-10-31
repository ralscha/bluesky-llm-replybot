package main

import (
	"strings"
	"unicode"

	"github.com/rivo/uniseg"
)

func countGraphemes(text string) int {
	return uniseg.GraphemeClusterCount(text)
}

func splitTextIntoChunks(text string, maxGraphemes int) []string {
	totalGraphemes := countGraphemes(text)

	if totalGraphemes <= maxGraphemes {
		return []string{text}
	}

	var chunks []string
	remainingText := text
	partNumber := 1

	for len(remainingText) > 0 {
		var reservedSpace int
		if partNumber == 1 {
			reservedSpace = 12
		} else {
			reservedSpace = 14
		}

		effectiveMax := max(maxGraphemes-reservedSpace, 50)
		chunk := extractChunk(remainingText, effectiveMax)

		if chunk == "" {
			chunk = extractChunkForced(remainingText, effectiveMax)
		}

		chunks = append(chunks, strings.TrimSpace(chunk))
		remainingText = strings.TrimSpace(remainingText[len(chunk):])
		partNumber++
	}

	return addContinuationMarkers(chunks)
}

func extractChunk(text string, maxGraphemes int) string {
	if countGraphemes(text) <= maxGraphemes {
		return text
	}

	gr := uniseg.NewGraphemes(text)
	var builder strings.Builder
	graphemeCount := 0
	lastBreakPoint := 0
	currentPos := 0

	for gr.Next() {
		graphemeCount++
		runes := gr.Runes()

		if graphemeCount <= maxGraphemes {
			for _, r := range runes {
				builder.WriteRune(r)
			}

			if len(runes) > 0 && (unicode.IsSpace(runes[0]) || unicode.IsPunct(runes[0])) {
				lastBreakPoint = builder.Len()
				currentPos = graphemeCount
			}
		} else {
			break
		}
	}

	result := builder.String()

	if lastBreakPoint > 0 && float64(currentPos) > float64(maxGraphemes)*0.7 {
		return result[:lastBreakPoint]
	}

	return result
}

func extractChunkForced(text string, maxGraphemes int) string {
	gr := uniseg.NewGraphemes(text)
	var builder strings.Builder
	graphemeCount := 0

	for gr.Next() {
		if graphemeCount >= maxGraphemes {
			break
		}
		runes := gr.Runes()
		for _, r := range runes {
			builder.WriteRune(r)
		}
		graphemeCount++
	}

	return builder.String()
}

func addContinuationMarkers(chunks []string) []string {
	if len(chunks) <= 1 {
		return chunks
	}

	total := len(chunks)
	marked := make([]string, len(chunks))

	for i, chunk := range chunks {
		partNum := i + 1
		switch partNum {
		case 1:
			marked[i] = strings.TrimSpace(chunk) + " (1/" + intToString(total) + ")"
		case total:
			marked[i] = "…(" + intToString(partNum) + "/" + intToString(total) + ") " + strings.TrimSpace(chunk)
		default:
			marked[i] = "…(" + intToString(partNum) + "/" + intToString(total) + ") " + strings.TrimSpace(chunk)
		}
	}

	return marked
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}
