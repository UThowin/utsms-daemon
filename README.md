UTSMS Daemon

A lightweight Go-based daemon that bridges MySQL ↔ SMSTools3 for reliable SMS sending, receiving, delivery-report parsing, and automated processing.

This project includes:

A Go SMS daemon (utsms-daemon)

A Debian package builder with systemd integration

Automatic .env config loading

Automatic service installation + startup

Clean packaging using dpkg-deb

⭐ Features

Reads outgoing SMS jobs from MySQL

Writes incoming SMS + delivery reports to MySQL

Generates .OUT files for SMSTools3

Parses SMSTools incoming files

Handles retry logic and delivery statuses

Uses .env configuration for DB + directories

Installs as a systemd service

Distributed as a .deb package, installable via dpkg

📦 Installation (Debian/Ubuntu)
1. Build the .deb package
./build.sh


This produces:

utsms-daemon_1.1_amd64.deb

2. Install the package
sudo dpkg -i utsms-daemon_1.1_amd64.deb

The installer will:

Create /etc/utsms-daemon.env

Create /opt/utsms-daemon working directory

Create smsd service user

Enable + start utsms-daemon.service

🛠 Configure Environment (.env)

Edit:

sudo nano /etc/utsms-daemon.env


Example:

SMS_DB_USER=smsuser
SMS_DB_PASS=changeme
SMS_DB_HOST=127.0.0.1
SMS_DB_PORT=3306
SMS_DB_NAME=smsdb
SMS_OUT_DIR=/var/spool/sms/outgoing
SMS_IN_DIR=/var/spool/sms/incoming


Restart service after changes:

sudo systemctl restart utsms-daemon

🛠 Configure smsd.conf

Edit:

sudo nano /etc/smsd.conf

Example:

devices = GSM1
outgoing = /var/spool/sms/outgoing
checked = /var/spool/sms/checked
incoming = /var/spool/sms/incoming
logfile = /var/log/smstools/smsd.log
infofile = /var/run/smstools/smsd.working
pidfile = /var/run/smstools/smsd.pid
failed = /var/spool/sms/failed
sent = /var/spool/sms/sent
stats = /var/log/smstools/smsd_stats
loglevel = 4
receive_before_send = no
autosplit = 3
store_received_pdu = 1
date_filename = 2
date_filename_format = %Y%m%d-%H%M%S
incoming_utf8 = yes
decode_unicode_text = yes

[GSM1]
init = AT+CNMI=1,2,2,1,1
device = /dev/ttyUSB2
incoming = yes
baudrate = 115200
report = yes
memory_start = 0
primary_memory = SM
secondary_memory = ME

Restart service after changes:

sudo systemctl restart smstools

🚀 Run & Manage Service

Check status:

systemctl status utsms-daemon


Stop / start:

sudo systemctl stop utsms-daemon
sudo systemctl start utsms-daemon


View logs:

sudo journalctl -u utsms-daemon -f

📁 Project Structure
📁 utsms-daemon/
│
├── main.go                 # Go source code
├── go.mod                  # Go module definition
├── go.sum                  # Module checksums
│
├── debian/                 # Debian package build folder
│   ├── DEBIAN/
│   │   ├── control         # Package metadata
│   │   ├── postinst        # Run after install (service setup)
│   │   └── prerm           # Run before uninstall
│   │
│   ├── usr/
│   │   └── bin/            # Binary install destination
│   │
│   ├── etc/                # Template configuration files
│   │   └── smsd.conf.sample
│   │
│   └── lib/
│       └── systemd/
│           └── system/
│               └── utsms-daemon.service
│
└── build.sh                # Automated .deb builder


💡 Development

Run directly without packaging:

go run main.go


Build binary manually:

go build -o utsms-daemon main.go

🔒 Security Notes

Do NOT commit /etc/utsms-daemon.env

Keep .env outside the repo (only template is included)

.deb installer uses correct permissions:

smsd user

/etc/utsms-daemon.env → 600

📝 License

MIT License (or whichever you prefer).
Feel free to modify and redistribute.
