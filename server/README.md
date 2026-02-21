
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
export LOG_PUBLIC_KEY='example.com/log/testdata+33d7b496+AeHTu4Q3hEIMHNqc6fASMsq3rKNx280NI+oO5xCFkkSx'
export LOG_DIR="$HOME/tessera-log-min"
mkdir -p "$LOG_DIR"

go run . --storage_dir="$LOG_DIR" --listen=":2025"