# Model Search Guidance

- `Message.MessageText` uses the PostgreSQL `tsvector` expression index `idx_messages_text_search`.
- Search messages with `to_tsvector('english', message_text) @@ plainto_tsquery('english', ?)`. Include `chat_id` filters when scoped to one chat.
- Do not use `ILIKE` as the primary message-search path.
