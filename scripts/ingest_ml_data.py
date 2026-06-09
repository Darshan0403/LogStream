# scripts/ingest_ml_data.py
import urllib.request
import json
import os
import re

# Configuration
LOGSTREAM_URL = "http://localhost:8090/ingest"
API_KEY = "dev-key"  # Change this if you updated your docker-compose!
BATCH_SIZE = 100

# Regex for extracting juicy metadata
IP_REGEX = re.compile(r'\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b')
USER_REGEX = re.compile(r'user\s+([^\s]+)\s+from')
PID_REGEX = re.compile(r'sshd\[(\d+)\]')

def send_to_logstream(batch):
    req = urllib.request.Request(LOGSTREAM_URL, data=json.dumps(batch).encode('utf-8'))
    req.add_header('Content-Type', 'application/json')
    req.add_header('X-API-Key', API_KEY)
    
    try:
        with urllib.request.urlopen(req) as response:
            pass
    except Exception as e:
        print(f"[Error] Failed to ingest batch: {e}")

def process_apache(filepath):
    print(f"🚀 Starting ETL for Apache from {filepath}...")
    batch = []
    
    with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
        for line in f:
            line = line.strip()
            if not line: continue

            # Extract the native Apache log level (e.g., [notice], [error])
            level = "INFO"
            if "[error]" in line: level = "ERROR"
            elif "[warn]" in line: level = "WARN"
            elif "[notice]" in line: level = "INFO"
            elif "[emerg]" in line or "[crit]" in line: level = "FATAL"

            batch.append({
                "level": level,
                "service": "apache-web",
                "message": line,
                "metadata": {"server_type": "httpd", "source": "LogHub_ML"}
            })

            if len(batch) >= BATCH_SIZE:
                send_to_logstream(batch)
                batch = []
        if batch: send_to_logstream(batch)
    print("✅ Finished Apache ingestion.")

def process_openssh(filepath):
    print(f"🚀 Starting ETL for OpenSSH from {filepath}...")
    batch = []
    
    with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
        for line in f:
            line = line.strip()
            if not line: continue

            # Baseline level
            level = "INFO"
            line_lower = line.lower()
            if "failed" in line_lower or "invalid" in line_lower or "disconnect" in line_lower:
                level = "WARN"
            if "fatal" in line_lower or "error" in line_lower:
                level = "ERROR"

            # Extract Metadata
            metadata = {"daemon": "sshd", "source": "LogHub_ML"}
            
            ip_match = IP_REGEX.search(line)
            if ip_match: metadata["attacker_ip"] = ip_match.group(0)
            
            user_match = USER_REGEX.search(line)
            if user_match: metadata["target_user"] = user_match.group(1)
            
            pid_match = PID_REGEX.search(line)
            if pid_match: metadata["process_id"] = int(pid_match.group(1))

            batch.append({
                "level": level,
                "service": "openssh-daemon",
                "message": line,
                "metadata": metadata
            })

            if len(batch) >= BATCH_SIZE:
                send_to_logstream(batch)
                batch = []
        if batch: send_to_logstream(batch)
    print("✅ Finished OpenSSH ingestion.")

if __name__ == "__main__":
    apache_path = "ml_datasets/Apache.log"
    ssh_path = "ml_datasets/SSH.log"

    if os.path.exists(apache_path):
        process_apache(apache_path)
    else:
        print(f"⚠️ Could not find {apache_path}")

    if os.path.exists(ssh_path):
        process_openssh(ssh_path)
    else:
        print(f"⚠️ Could not find {ssh_path}")