# Environment Variables

## envConfig

 - Service identity configuration.
   - `SERVICE_NAME` - Runtime service name. If SERVICE_NAME is set, it overrides the name
derived from Module.Name(). If SERVICE_NAME is empty, the module name is
used as the runtime name.
   - `SERVICE_ENV` (default: `development`) - Deployment environment, such as "development", "staging", or
"production".
   - `SERVICE_VERSION` (default: `dev`) - Service version string, typically set by the build pipeline.
 - HTTP server configuration.
   - `HTTP_ADDR` (default: `:8080`) - Listen address for the HTTP server, such as ":8080".
   - `HTTP_SHUTDOWN_TIMEOUT` (default: `10s`) - Maximum duration to wait for the HTTP server to complete in-flight
requests during graceful shutdown.
 - Postgres database configuration.
   - `DATABASE_URL` - Postgres connection URL. It is required when the Postgres capability
connects to the database.
   - `DATABASE_MAX_CONNS` (default: `10`) - Maximum number of connections in the pgx pool.
   - `DATABASE_MIN_CONNS` (default: `0`) - Minimum number of connections in the pgx pool.
   - `DATABASE_CONNECT_TIMEOUT` (default: `5s`) - Timeout for establishing a Postgres connection.
 - Logger configuration.
   - `LOG_FORMAT` (default: `text`) - Log output format. Valid values are "text" and "json".
   - `LOG_COLOR` (default: `auto`) - ANSI colorization for text log output. Valid values are "on" (always
colorize), "off" (never colorize), and "auto" (colorize only when output
is a terminal). Colorization is only applied when Format is "text".
