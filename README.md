# Renotify

Renotify provides tools and services to simplify sending notifications to
multiple devices. It is aimed at supporting software development using agents
and waiting for user responses from multiple active pipelines.

Tooling includes:
- Android application to receive notifications.
- Go based CLI to send notifications and receive responses.
  - single-shot mode to send a toast
  - interactive mode to send a notification and wait for a response
  - history mode to view past notifications and responses
