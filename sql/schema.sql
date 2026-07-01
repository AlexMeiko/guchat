CREATE TABLE users
(
    id            BIGINT PRIMARY KEY AUTO_INCREMENT,
    username      VARCHAR(64)  NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(32)  NOT NULL DEFAULT 'user',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_users_username (username)
) ENGINE = InnoDB;

CREATE TABLE refresh_tokens
(
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    jti        CHAR(32)  NOT NULL,
    user_id    BIGINT    NOT NULL,
    expires_at DATETIME  NOT NULL,
    revoked_at DATETIME  NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_refresh_tokens_jti (jti),
    KEY idx_refresh_tokens_user_id (user_id),
    CONSTRAINT fk_refresh_tokens_user_id
        FOREIGN KEY (user_id) REFERENCES users (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;

CREATE TABLE conversations
(
    id         CHAR(36) PRIMARY KEY,
    user_id    BIGINT       NOT NULL,
    title      VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_conversations_user_updated_id (user_id, updated_at, id),
    CONSTRAINT fk_conversations_user_id
        FOREIGN KEY (user_id) REFERENCES users (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;

CREATE TABLE messages
(
    id                CHAR(36) PRIMARY KEY,
    conversation_id   CHAR(36)     NOT NULL,
    role              VARCHAR(16)  NOT NULL,
    content           MEDIUMTEXT   NOT NULL,
    reasoning_content MEDIUMTEXT   NOT NULL,
    summary_content   MEDIUMTEXT   NOT NULL,
    has_summary       BOOLEAN      NOT NULL DEFAULT FALSE,
    status            VARCHAR(16)  NOT NULL DEFAULT 'done',
    error_message     TEXT         NOT NULL,
    seq               INT          NOT NULL,
    created_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_messages_conversation_seq (conversation_id, seq),
    KEY idx_messages_summary_anchor (conversation_id, has_summary, seq),
    CONSTRAINT fk_messages_conversation_id
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;

CREATE TABLE models
(
    id         BIGINT PRIMARY KEY AUTO_INCREMENT,
    name       VARCHAR(64)  NOT NULL,
    provider   VARCHAR(32)  NOT NULL,
    model_key  VARCHAR(64)  NOT NULL,
    base_url   VARCHAR(255) NOT NULL,
    api_key    VARCHAR(255) NOT NULL,
    extra_body TEXT         NOT NULL,
    is_enabled BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE = InnoDB;

CREATE TABLE tool_calls
(
    id                   BIGINT PRIMARY KEY AUTO_INCREMENT,
    conversation_id      CHAR(36)     NOT NULL,
    assistant_message_id CHAR(36)     NOT NULL,
    provider_call_id     VARCHAR(128) NOT NULL DEFAULT '',
    tool_name            VARCHAR(128) NOT NULL,
    arguments_json       MEDIUMTEXT   NOT NULL,
    result_json          MEDIUMTEXT   NOT NULL,
    status               VARCHAR(16)  NOT NULL DEFAULT 'pending',
    error_message        TEXT         NOT NULL,
    round                INT          NOT NULL,
    seq                  INT          NOT NULL,
    started_at           DATETIME(3)  NULL,
    finished_at          DATETIME(3)  NULL,
    created_at           TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_tool_calls_message_round_seq (assistant_message_id, round, seq),
    KEY idx_tool_calls_conversation_id (conversation_id),

    CONSTRAINT fk_tool_calls_conversation_id
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
            ON DELETE CASCADE,
    CONSTRAINT fk_tool_calls_assistant_message_id
        FOREIGN KEY (assistant_message_id) REFERENCES messages (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;

CREATE TABLE generation_rounds
(
    id                     BIGINT PRIMARY KEY AUTO_INCREMENT,
    conversation_id        CHAR(36)     NOT NULL,
    assistant_message_id   CHAR(36)     NOT NULL,
    round                  INT          NOT NULL,
    content_start_offset   INT          NOT NULL DEFAULT 0,
    content_end_offset     INT          NOT NULL DEFAULT 0,
    reasoning_start_offset INT          NOT NULL DEFAULT 0,
    reasoning_end_offset   INT          NOT NULL DEFAULT 0,
    created_at             TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_generation_rounds_message_round (assistant_message_id, round),
    KEY idx_generation_rounds_conversation_id (conversation_id),

    CONSTRAINT fk_generation_rounds_conversation_id
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
            ON DELETE CASCADE,
    CONSTRAINT fk_generation_rounds_assistant_message_id
        FOREIGN KEY (assistant_message_id) REFERENCES messages (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;

CREATE TABLE memory_items
(
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    owner_user_id   BIGINT        NULL,
    conversation_id CHAR(36)      NULL,
    scope           VARCHAR(32)   NOT NULL DEFAULT 'user',
    category        VARCHAR(32)   NOT NULL,
    origin          VARCHAR(32)   NOT NULL,
    source_type     VARCHAR(32)   NOT NULL DEFAULT 'none',
    source_ref      TEXT          NULL,
    source_title    VARCHAR(256)  NULL,
    content         MEDIUMTEXT    NOT NULL,
    metadata_json   TEXT          NOT NULL,
    confidence      DECIMAL(4, 3) NOT NULL DEFAULT 0.7,
    expires_at      DATETIME(3)   NULL,
    status          VARCHAR(16)   NOT NULL DEFAULT 'active',
    created_at      TIMESTAMP(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    KEY idx_memory_owner_status_updated (owner_user_id, status, updated_at, id),
    KEY idx_memory_owner_status_category_updated (owner_user_id, status, category, updated_at, id),
    KEY idx_memory_owner_status_scope_conversation_updated (owner_user_id, status, scope, conversation_id, updated_at, id),
    KEY idx_memory_scope_status_updated (scope, status, updated_at, id),
    KEY idx_memory_conversation (conversation_id),
    KEY idx_memory_expires_at (expires_at),

    CONSTRAINT fk_memory_owner_user_id
        FOREIGN KEY (owner_user_id) REFERENCES users (id)
            ON DELETE CASCADE,
    CONSTRAINT fk_memory_conversation_id
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;
