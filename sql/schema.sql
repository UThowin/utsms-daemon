CREATE TABLE IF NOT EXISTS sms_messages (
    id INT AUTO_INCREMENT PRIMARY KEY,
    direction ENUM('Incoming', 'Outgoing') NOT NULL,
    phone VARCHAR(32) NOT NULL,
    text TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'Pending',
    modem VARCHAR(32) DEFAULT 'GSM1',
    sent_time DATETIME NULL,
    delivered_time DATETIME NULL,
    report_time DATETIME NULL,
    created_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Recommended indexes
CREATE INDEX idx_phone ON sms_messages(phone);
CREATE INDEX idx_direction ON sms_messages(direction);
CREATE INDEX idx_status ON sms_messages(status);
CREATE INDEX idx_created_time ON sms_messages(created_time);
