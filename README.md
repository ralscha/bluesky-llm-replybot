# Bluesky LLM Reply Bot

A Go bot that watches Bluesky mentions, queues them in PostgreSQL, generates an Eino-powered LLM response, and posts the reply back to Bluesky.

## Features

- Ingests unread Bluesky mention notifications
- Stores work in a PostgreSQL-backed queue and history table
- Uses Eino's chat model interface with OpenAI-compatible model configuration
- Tracks cached input, uncached input, and output token spend against a daily budget
- Defers work when the daily budget is exhausted and replies with the hours until reset
- Retries failed LLM generation and reply sending before recording a failure
- Splits long replies into Bluesky reply threads using grapheme-aware text splitting
- Runs database migrations on startup

## Requirements

- Go 1.26.4 or newer
- Docker with Docker Compose
- Task, for the included `Taskfile.yml` shortcuts

## Configuration

Copy `.env.example` to `.env` and fill in the values:

```sh
cp .env.example .env
```

Required Bluesky settings:

- `BLUESKY_IDENTIFIER`: the bot account handle or DID used to sign in
- `BLUESKY_PASSWORD`: a Bluesky app password
- `BLUESKY_HOST`: usually `https://bsky.social`
- `BOT_HANDLE`: the mention text the bot should respond to, for example `@your.handle.example`

Required LLM settings:

- `LLM_PROVIDER`: `openai` or `openai-compatible`
- `LLM_API_KEY`: API key for the configured model provider
- `LLM_MODEL`: model name passed to Eino
- `LLM_BASE_URL`: optional OpenAI-compatible base URL for non-OpenAI providers
- `LLM_TEMPERATURE`: optional, defaults to `0.7`
- `LLM_MAX_OUTPUT_TOKENS`: hard maximum output tokens per LLM call; used for pre-request cost reservation
- `LLM_REQUESTS_PER_MINUTE`: maximum LLM generation requests per minute; set to `0` to disable request rate limiting

Spending controls:

- `LLM_PRICE_INPUT_CACHE_PER_MILLION`: price per million cached input tokens
- `LLM_PRICE_INPUT_MISS_PER_MILLION`: price per million uncached input tokens
- `LLM_PRICE_OUTPUT_PER_MILLION`: price per million output tokens
- `LLM_DAILY_SPENDING_LIMIT`: daily budget in the same currency; set to `0` to disable enforcement. If this is greater than `0`, at least one price must also be greater than `0`.

Before each LLM call, the bot reserves the worst-case cost using uncached input tokens plus `LLM_MAX_OUTPUT_TOKENS`. If that reservation would exceed the daily budget, no LLM call is made. The bot sends a reply saying the message will be processed after the next UTC daily reset and includes the approximate hours until then. The original queue item remains deferred and is processed after that reset.

Database settings:

- `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `DB_SSLMODE`
- `MAX_RETRIES`: optional, defaults to `3`

## Running Locally

Start PostgreSQL:

```sh
task db:up
```

Run the bot without building a binary:

```sh
task run:dev
```

Build and run the binary:

```sh
task build
task run
```

The default build output is `bin/app` on Unix-like systems and `bin/app.exe` on Windows.

## Development

```sh
go test ./...
go vet ./...
task sqlc:generate
```

`task sqlc:generate` regenerates the SQL bindings from `internal/database/queries` using `sqlc/sqlc:1.30.0` in Docker.

Useful database commands:

```sh
task db:logs
task db:down
```

## Deployment

A Debian systemd example is available in `systemd/`. See `systemd/INSTALL.md` for installing the built binary, `.env`, Docker Compose database, and service unit under `/opt/bluesky-replybot`.

## Demo

Post on Bluesky mentioning the configured `BOT_HANDLE` and the bot will queue the mention, generate an LLM response, and reply from the bot account.
