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
    status            VARCHAR(16)  NOT NULL DEFAULT 'done',
    error_message     TEXT         NOT NULL,
    seq               INT          NOT NULL,
    created_at        TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_messages_conversation_seq (conversation_id, seq),
    CONSTRAINT fk_messages_conversation_id
        FOREIGN KEY (conversation_id) REFERENCES conversations (id)
            ON DELETE CASCADE
) ENGINE = InnoDB;
