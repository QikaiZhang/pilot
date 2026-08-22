


CREATE TABLE conversation_history (
       id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
       message_id VARCHAR(128) NOT NULL,
       user_id VARCHAR(128) NOT NULL,
       session_id VARCHAR(128) NOT NULL,
       role VARCHAR(16) NOT NULL,
       content LONGTEXT NOT NULL,
       metadata JSON NULL,
       created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
       UNIQUE KEY uk_history_message (user_id, session_id, message_id),
       KEY idx_history_session_created (user_id, session_id, created_at, id)
     );
