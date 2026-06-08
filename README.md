# Bluesky LLM Reply Bot

A Go bot that monitors Bluesky mentions and generates automated replies using an LLM.

## Features

- Ingests posts from Bluesky notifications
- Queues posts for processing into PostgreSQL
- Generates LLM-powered replies with Google Gemini Flash and Flash-Lite
- Tracks model and Google Search grounding limits to avoid API throttling
- Splits long replies into Bluesky threads

## Requirements

- Go 1.26+
- Docker with Docker Compose
- Task (optional, but used by the included `Taskfile.yml`)

## Configuration

Copy `.env.example` to `.env` and fill in the Bluesky, Gemini, and PostgreSQL settings.
`BLUESKY_PASSWORD` should be a Bluesky app password.

## Running

Start PostgreSQL:

```sh
task db:up
```

Run the bot:

```sh
task run:dev
```

Build a binary:

```sh
task build
```

The application runs database migrations on startup.

## Development

```sh
go test ./...
go vet ./...
task sqlc:generate
```

`task sqlc:generate` regenerates the SQL bindings with sqlc in Docker.

## Demo

Write a post on Bluesky mentioning @llm.ras.ch and see the bot reply with a LLM-generated response!
