# Bluesky LLM Reply Bot

A Go bot that monitors Bluesky posts and generates automated replies using a LLM.

## Features

- Ingests posts from Bluesky notifications
- Queues posts for processing into PostgreSQL
- Generates LLM-powered replies with Google Gemini Flash and Flash-Lite
- Rate limiting to avoid API throttling
