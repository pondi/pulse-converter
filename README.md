# PaperPulse Office Document Converter

Go-based microservice for converting office documents (.docx, .xlsx, .pptx, etc.) to PDF/A format using Gotenberg and Redis job queue.

## Architecture

- **Redis Queue**: Jobs are pushed to `conversion:pending` queue by Laravel
- **Worker Pool**: Multiple Go workers poll Redis using BRPOPLPUSH for atomic job claiming
- **Gotenberg**: LibreOffice-based conversion service running in daemon mode
- **S3**: File downloads and uploads
- **PostgreSQL**: Conversion status tracking

## Components

- `main.go` - Entry point, starts worker pool and recovery loop
- `config/config.go` - Environment configuration loader
- `models/conversion_job.go` - Job payload structure
- `services/gotenberg.go` - Gotenberg HTTP client (PDF/A conversion)
- `services/s3.go` - S3 download/upload operations
- `services/database.go` - PostgreSQL status updates
- `worker/pool.go` - Worker pool management and job processing

## Environment Variables

Required variables (set in .env or docker-compose):

```env
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_CONVERSION_DB=3
GOTENBERG_URL=http://gotenberg:3000
AWS_BUCKET=paperpulse
AWS_DEFAULT_REGION=us-east-1
AWS_ACCESS_KEY_ID=
AWS_SECRET_ACCESS_KEY=
DB_HOST=postgres
DB_PORT=5432
DB_DATABASE=paperpulse
DB_USERNAME=paperpulse
DB_PASSWORD=secret
DB_SSLMODE=disable
CONVERSION_WORKER_COUNT=3
CONVERSION_TIMEOUT=120
CONVERSION_MAX_RETRIES=3
```

## Building

```bash
# Build locally
cd deploy/converter
go mod download
go build -o converter .

# Build Docker image
docker build -f deploy/Dockerfile.converter -t paperpulse-converter .
```

## Running

### Docker Compose (Recommended)
```bash
docker-compose up -d gotenberg converter
```

### Standalone
```bash
export REDIS_ADDR=localhost:6379
export DB_HOST=localhost
export DB_PORT=5432
export DB_DATABASE=paperpulse
export DB_USERNAME=paperpulse
export DB_PASSWORD=secret
export DB_SSLMODE=disable
export GOTENBERG_URL=http://localhost:3000
export AWS_BUCKET=...
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...

./converter
```

## Monitoring

### Check Worker Status
```bash
docker-compose logs -f converter
```

### Check Redis Queues
```bash
redis-cli -h localhost -p 6379 -n 3
> LLEN conversion:pending
> LLEN conversion:processing
> LLEN conversion:failed
```

### Check Database
```sql
SELECT status, COUNT(*) FROM file_conversions GROUP BY status;
SELECT * FROM file_conversions WHERE status = 'failed' ORDER BY created_at DESC LIMIT 10;
```

## Error Handling

- **Retry Logic**: Exponential backoff (2s, 4s, 8s... max 30s)
- **Max Retries**: 3 attempts before moving to failed queue
- **Stale Job Recovery**: Every 5 minutes, requeues jobs stuck in processing > 5min
- **Graceful Failure**: If conversion fails after retries, Laravel continues with original file

## Scaling

### Docker Compose (Development)
Horizontal scaling with shared Gotenberg:

```bash
docker-compose up -d --scale converter=5
```

Each worker safely claims jobs using Redis BRPOPLPUSH atomic operation.

**Note**: In docker-compose, all converters share one Gotenberg instance. This works for development but may bottleneck in production.

### Kubernetes (Production)
Uses **sidecar pattern** - each converter Pod gets its own Gotenberg instance:

```bash
kubectl scale deployment paperpulse-converter --replicas=5 -n paperpulse
```

This creates 5 Pods, each with 2 containers:
- 5x Go converter workers
- 5x Gotenberg instances (one per worker)

**Why sidecar?** Gotenberg (LibreOffice) does the heavy lifting. Giving each converter its own Gotenberg eliminates contention and bottlenecks.

See `deploy/k8s/README.md` for full Kubernetes deployment details.

## Supported Formats

- **Word**: .doc, .docx, .odt, .rtf
- **Excel**: .xls, .xlsx, .ods  
- **PowerPoint**: .ppt, .pptx, .odp
- **Other**: .txt, .html

All output files are PDF/A-1a format for archiving compliance.
