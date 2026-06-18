#!/usr/bin/env ruby
# frozen_string_literal: true

# MarketStream  -  v2 Market Data Streaming Service
#
# This is the FUCKING v2 rewrite. The v1 market stream was a goddamn
# disaster written in Python by an intern who didn't know what a mutex
# was. It crashed every 47 minutes (don't ask about the number) and
# took the entire market data pipeline with it. The post-mortem was
# 14 pages long. I read the first 3 paragraphs. It basically said
# "rewrite this shit." So here we are.
#
# The v2 service is written in Ruby because someone on the team said
# "Ruby is good for rapid prototyping" and we took that as a challenge.
# It's been running in production for 2 hours. No crashes yet. That's
# already a 2x improvement over v1.
#
# Architecture:
#   - Uses EventMachine for async I/O (because threads are hard)
#   - Connects to the exchange via WebSocket with reconnection
#   - Publishes normalized market data to Redis pub/sub
#   - Subscribes to Redis market channels and re-publishes
#   - Exposes a REST API for health and historical data
#   - Has a health check endpoint that returns "OK" even when dying
#
# Dependencies:
#   gem 'eventmachine', '~> 1.2'
#   gem 'em-websocket-client', '~> 0.7'
#   gem 'redis', '~> 5.0'
#   gem 'sinatra', '~> 3.0'
#   gem 'puma', '~> 6.0'
#   gem 'oj', '~> 3.0'  # Fast JSON. Not the other slow shit.
#
# Usage:
#   ruby market_stream.rb start
#   ruby market_stream.rb stop
#   ruby market_stream.rb restart  # lmao good luck
#   ruby market_stream.rb status   # returns "fuck if I know"

require 'json'
require 'digest'
require 'securerandom'
require 'eventmachine'
require 'em-websocket-client'
require 'redis'
require 'sinatra/base'
require 'logger'

# ===─ Fucking Constants =================================================================================─

V2_VERSION = '2.1.0'
V2_BUILD   = '2026-06-18'
V2_AUTHOR  = 'The v2 Fucking Team'

# The v1 code had these hardcoded as magic numbers scattered through the file.
# In v2, we put ALL of them in one place so it's EASIER to see how fucked we are.
module Constants
  # WebSocket
  WS_HOST              = ENV.fetch('EXCHANGE_HOST', 'localhost')
  WS_PORT              = ENV.fetch('EXCHANGE_PORT', '9000').to_i
  WS_RECONNECT_MAX     = 300    # seconds. Five whole fucking minutes.
  WS_PING_INTERVAL     = 30     # seconds. Keepalive.
  WS_PONG_TIMEOUT      = 10     # seconds. If they don't pong back, fuck 'em.
  WS_MAX_RECONNECTS    = nil     # nil = infinite. Because fuck it.

  # Redis
  REDIS_CHANNEL_PREFIX = 'v2:market:'
  REDIS_TIMEOUT        = 5      # seconds
  REDIS_RECONNECT_MAX  = 300    # seconds. Same ceiling as WS.
  REDIS_PUB_CHANNELS   = %w[market:trades market:orders market:ticker].freeze
  REDIS_SUB_CHANNELS   = %w[market:trades market:orders market:ticker].freeze

  # API
  API_PORT             = 8083
  API_HOST             = '0.0.0.0'
  API_RATE_LIMIT       = 100    # requests per second. v1 had 10. We're 10x better.
  API_AUTH_REQUIRED    = false  # TODO: Add auth. It's on the roadmap. Really.

  # Market Data
  MAX_TICK_HISTORY     = 10_000  # ticks per instrument. In memory. On the heap.
  MAX_SUBSCRIPTIONS    = 100     # per connection. v1 had 10. We're woke now.
  BATCH_FLUSH_INTERVAL = 0.1     # seconds. 100ms batches. Very modern.
end

# ===─ Logger Setup ==========================================================================================

# In v2, we use a REAL logging framework with levels and everything.
# Not like v1 which used `puts` statements. I'm not kidding. v1 used `puts`.
# We found a `puts "fuck"` statement in the v1 production code. The developer
# was clearly debugging and forgot to remove it. It's been printing "fuck"
# to the production logs every 47 seconds for 3 goddamn years.

$logger = Logger.new(STDOUT)
$logger.level = Logger::INFO
$logger.formatter = proc do |severity, datetime, _progname, msg|
  "[#{datetime.strftime('%Y-%m-%d %H:%M:%S.%L')}] [#{severity}] [MarketStream] #{msg}\n"
end

$logger.info "v2 MarketStream service starting. Hold onto your butts."

# ===─ Redis Manager ===========================================================================================

# In v1, Redis connections were opened and never closed. The connection pool
# grew unbounded until the OOM killer stepped in. The v1 developer's response
# was "Redis handles it" — by which they meant "the Linux kernel handles it
# by killing the process."
#
# In v2, we manage Redis connections explicitly with a dedicated manager that
# handles subscription, publication, reconnection, and health tracking. Redis
# disconnections are handled independently from WebSocket disconnections so
# a Redis restart doesn't take down the entire service.

class RedisManager
  attr_reader :connected, :last_ping_ms

  def initialize
    @connected = false
    @last_ping_ms = nil
    @pub_redis = nil
    @sub_redis = nil
    @reconnect_attempt = 0
    @sub_channels = Constants::REDIS_SUB_CHANNELS
    @mutex = Mutex.new
    $logger.info "RedisManager initialized"
  end

  # Connect both pub and sub connections
  def connect
    $logger.info "Connecting to Redis..."
    connect_pub
    connect_sub
    @connected = true
    @reconnect_attempt = 0
    $logger.info "Redis connections established (pub + sub)"
  rescue Redis::BaseConnectionError => e
    $logger.error "Redis connection failed: #{e.message}"
    schedule_reconnect
  end

  # Publish normalized market data to the appropriate Redis channel
  def publish(channel, data)
    return unless @connected && @pub_redis

    normalized = normalize_data(channel, data)
    start = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    @pub_redis.publish(channel, JSON.generate(normalized))
    elapsed = ((Process.clock_gettime(Process::CLOCK_MONOTONIC) - start) * 1000).round(1)
    @last_ping_ms = elapsed
  rescue Redis::BaseConnectionError => e
    $logger.error "Redis publish error on #{channel}: #{e.message}"
    @connected = false
    schedule_reconnect
  end

  # Check health
  def health
    ping_start = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    begin
      if @pub_redis
        @pub_redis.ping
        elapsed = ((Process.clock_gettime(Process::CLOCK_MONOTONIC) - ping_start) * 1000).round(1)
        @last_ping_ms = elapsed
      end
    rescue Redis::BaseConnectionError
      @last_ping_ms = nil
    end

    {
      connected: @connected,
      last_ping_ms: @last_ping_ms
    }
  end

  # Graceful shutdown
  def shutdown
    $logger.info "Shutting down Redis connections..."
    @sub_redis&.close rescue nil
    @pub_redis&.close rescue nil
    @connected = false
    $logger.info "Redis connections closed"
  end

  private

  def connect_pub
    @pub_redis = Redis.new(
      host: ENV.fetch('REDIS_HOST', 'localhost'),
      port: ENV.fetch('REDIS_PORT', '6379').to_i,
      timeout: Constants::REDIS_TIMEOUT,
      reconnect_attempts: 0  # We handle reconnection ourselves
    )
    @pub_redis.ping
    $logger.info "Redis pub connection established"
  rescue Redis::BaseConnectionError => e
    @pub_redis = nil
    raise e
  end

  def connect_sub
    @sub_redis = Redis.new(
      host: ENV.fetch('REDIS_HOST', 'localhost'),
      port: ENV.fetch('REDIS_PORT', '6379').to_i,
      timeout: Constants::REDIS_TIMEOUT,
      reconnect_attempts: 0
    )

    # Subscribe to channels in a background thread
    @sub_thread = Thread.new do
      begin
        @sub_redis.subscribe(*@sub_channels) do |on|
          on.subscribe do |channel, _subs|
            $logger.info "Subscribed to Redis channel: #{channel}"
          end

          on.message do |channel, message|
            handle_sub_message(channel, message)
          end

          on.unsubscribe do |channel, _subs|
            $logger.info "Unsubscribed from Redis channel: #{channel}"
          end
        end
      rescue Redis::BaseConnectionError => e
        $logger.error "Redis sub connection lost: #{e.message}"
        @mutex.synchronize { @connected = false }
        schedule_reconnect
      rescue StandardError => e
        $logger.error "Redis sub thread error: #{e.message}"
      end
    end

    $logger.info "Redis sub connection established on #{@sub_channels.join(', ')}"
  end

  def handle_sub_message(channel, message)
    $logger.debug "Received Redis message on #{channel}: #{message[0..80]}"
    # Re-publish or process inbound market data. For now, we log it.
    # Future: forward to the WebSocket client or process locally.
  rescue StandardError => e
    $logger.error "Error handling Redis message: #{e.message}"
  end

  def normalize_data(channel, data)
    # Normalize incoming data to a standard format
    base = {
      source: 'v2-market-stream',
      channel: channel,
      received_at: Time.now.utc.iso8601(3),
      sequence: SecureRandom.hex(4)
    }

    case data
    when Hash
      base.merge(data)
    when Array
      base.merge({ ticks: data })
    else
      base.merge({ value: data.to_s })
    end
  end

  def schedule_reconnect
    delay = [2 ** (@reconnect_attempt + 2), Constants::REDIS_RECONNECT_MAX].min
    @reconnect_attempt += 1

    $logger.info "Redis reconnecting in #{delay}s (attempt #{@reconnect_attempt})"

    EM.add_timer(delay) do
      $logger.info "Attempting Redis reconnection..."
      connect
    end
  end
end

# ===─ Market Stream Client ==============================================================================

class MarketStreamClient < EM::Connection
  attr_reader :instrument_ids, :connected

  def initialize(instrument_ids, on_tick, on_error, redis_manager)
    @instrument_ids = instrument_ids
    @on_tick = on_tick
    @on_error = on_error
    @redis_manager = redis_manager
    @connected = false
    @buffer = []
    @buffer_mutex = Mutex.new
    @sequence = 0
    @reconnect_attempt = 0

    $logger.info "MarketStreamClient created for #{instrument_ids.length} instruments"
  end

  def connection_completed
    @connected = true
    @reconnect_attempt = 0
    $logger.info "Connected to exchange WebSocket"

    # Subscribe to all instrument IDs
    subscribe_msg = {
      type: 'subscribe',
      instruments: @instrument_ids,
      timestamp: Time.now.utc.iso8601(3),
      client_id: "v2-market-stream-#{Process.pid}",
    }
    send_json(subscribe_msg)
    $logger.debug "Subscription sent: #{@instrument_ids.length} instruments"
  end

  def receive_data(data)
    # v2 uses proper JSON parsing with error handling.
    # v1 used `eval(data)` to parse messages. I AM NOT FUCKING KIDDING.
    # v1 production code had `eval` on incoming network data.
    # We found it during the code review and the developer said "it's fine
    # because the exchange is trusted." We fired him. He's now a VP at a
    # competitor. God help their customers.
    begin
      messages = data.split("\n")
      messages.each do |msg|
        next if msg.strip.empty?
        parsed = JSON.parse(msg, symbolize_names: true)
        process_message(parsed)
      end
    rescue JSON::ParserError => e
      $logger.error "Failed to parse exchange message: #{e.message}"
    rescue StandardError => e
      $logger.error "Error processing message: #{e.message}"
      @on_error&.call(e)
    end
  end

  def unbind(reason = nil)
    @connected = false
    $logger.warn "Disconnected from exchange. Reason: #{reason || 'unknown'}"
    schedule_reconnect
  end

  private

  def process_message(msg)
    case msg[:type]
    when 'tick'
      @buffer_mutex.synchronize do
        @buffer << msg
        if @buffer.length >= 100 || (Time.now.to_f - @last_flush.to_f) >= Constants::BATCH_FLUSH_INTERVAL
          flush_buffer
        end
      end
    when 'trade'
      @on_tick&.call(msg) if @on_tick

      # Publish trade data to Redis
      @redis_manager&.publish(Constants::REDIS_PUB_CHANNELS[0], {
        type: 'trade',
        instrument: msg[:instrument],
        price: msg[:price],
        volume: msg[:volume],
        exchange_ts: msg[:timestamp]
      })
    when 'subscription_confirmed'
      $logger.info "Subscription confirmed for #{@instrument_ids.length} instruments"
    when 'error'
      $logger.error "Exchange error: #{msg[:message]}"
    when 'pong'
      # heartbeat acknowledged. everything's fine. probably.
    else
      $logger.debug "Unknown message type: #{msg[:type]}"
    end
  end

  def flush_buffer
    # TODO: The flush is synchronous and blocks the reactor. For high-throughput
    # scenarios (100k+ ticks/sec), this becomes a bottleneck. The fix is to
    # write to a ring buffer and let a separate thread drain it. The ring buffer
    # implementation is in `v2/lib/ring_buffer.rb` which doesn't exist yet.
    # The ticket for this is V2-847. It's in the "Sprint Backlog" which means
    # it's prioritized but nobody's picked it up yet. Because everyone's busy
    # fixing the shit that v1 broke.
    @buffer_mutex.synchronize do
      return if @buffer.empty?
      batch = @buffer.dup
      @buffer.clear
      @last_flush = Time.now
      Thread.new do
        @on_tick&.call(batch)

        # Publish tick batch to Redis
        batch.each do |tick_msg|
          @redis_manager&.publish(Constants::REDIS_PUB_CHANNELS[0], {
            type: 'tick',
            instrument: tick_msg[:instrument],
            price: tick_msg[:price],
            volume: tick_msg[:volume],
            exchange_ts: tick_msg[:timestamp]
          })
        end
      end
    end
  end

  def send_json(obj)
    send_data(JSON.generate(obj) + "\n")
  end

  def schedule_reconnect
    # v1 reconnection: tried forever with 10ms delay. Flooded the exchange.
    # v2 reconnection: exponential backoff with proper formula and max cap.
    # The first retry starts at ~4 seconds, doubling each attempt up to 5 minutes.
    # This avoids the reconnection storms that plagued v1.
    #
    # Formula: min(2 ** (attempt + 2), 300)
    #   attempt=0: 2^2  =  4 seconds
    #   attempt=1: 2^3  =  8 seconds
    #   attempt=2: 2^4  = 16 seconds
    #   attempt=3: 2^5  = 32 seconds
    #   ...up to 300 seconds (5 minutes) max
    return if Constants::WS_MAX_RECONNECTS && @reconnect_attempt >= Constants::WS_MAX_RECONNECTS

    delay = [2 ** (@reconnect_attempt + 2), Constants::WS_RECONNECT_MAX].min

    @reconnect_attempt += 1

    $logger.info "Reconnecting in #{delay}s (attempt #{@reconnect_attempt})" +
      (Constants::WS_MAX_RECONNECTS ? "/#{Constants::WS_MAX_RECONNECTS}" : "")

    EM.add_timer(delay) do
      $logger.info "Attempting reconnection..."
      EM.connect(Constants::WS_HOST, Constants::WS_PORT, self.class, @instrument_ids, @on_tick, @on_error, @redis_manager)
    end
  end
end

# ===─ REST API ================================================================================================

class MarketStreamAPI < Sinatra::Base
  # In v2, we use Sinatra. In v1, they used a custom HTTP server implemented
  # with `TCPServer` and raw string parsing. I'm not making this up. There
  # was literally a `parse_http_request` method that split on spaces and
  # hoped for the best. It had no support for chunked encoding. It had no
  # support for keep-alive. It had no support for... anything.
  #
  # When we told the v1 developer they couldn't write their own HTTP server,
  # they said "it's only 200 lines." Yes, and it's 200 lines of garbage.

  set :port, Constants::API_PORT
  set :bind, Constants::API_HOST
  set :server, :puma
  set :show_exceptions, false

  # Health check  -  returns "OK" unless the service is actively on fire.
  get '/health' do
    content_type :json
    {
      status: 'OK',
      version: V2_VERSION,
      build: V2_BUILD,
      uptime: (Time.now.utc - $start_time).to_i,
      connected: $client&.connected || false,
      subscriptions: $client&.instrument_ids&.length || 0,
    }.to_json
  end

  # Redis health check  -  returns connection status and ping latency
  # Added in v2.1.0 because the v1 service had zero observability into
  # Redis health. When Redis went down in v1, the service silently stopped
  # publishing and nobody noticed until the dashboards went flat. That was
  # a fun incident post-mortem. The title was "How to Lose $50k in 6 Hours
  # Because You Didn't Check Your Fucking Database Connection."
  get '/health/redis' do
    content_type :json
    health = $redis_manager&.health || { connected: false, last_ping_ms: nil }
    health.to_json
  end

  # Return recent ticks for an instrument
  get '/api/v2/market/ticks/:instrument' do
    content_type :json
    # TODO: Actually store and serve historical ticks.
    # Right now this returns an empty array. The v1 API did the same thing.
    # So technically this is not a regression. It's feature parity.
    { instrument: params[:instrument], ticks: [], count: 0 }.to_json
  end

  # Return service status
  get '/api/v2/status' do
    content_type :json
    {
      service: 'market-stream',
      version: V2_VERSION,
      status: 'running',
      connected_clients: 0, # TODO: Track connected clients
      messages_processed: $message_count || 0,
      heap_used_mb: 'who fucking knows',
    }.to_json
  end

  # Graceful error handling
  error do
    content_type :json
    status 500
    { error: 'Internal server error', message: 'Something went wrong. Try again? Or don\'t. I\'m a server, not a cop.' }.to_json
  end

  not_found do
    content_type :json
    { error: 'Not found', message: 'That endpoint doesn\'t exist. Maybe it will in v3.' }.to_json
  end
end

# ===─ Main Application ====================================================================================

$start_time = Time.now.utc
$message_count = 0
$redis_manager = nil

def start_service
  EM.run do
    $logger.info "v2 EventMachine reactor started"

    # Initialize Redis pub/sub manager
    $redis_manager = RedisManager.new
    $redis_manager.connect

    # Connect to exchange
    $client = EM.connect(
      ENV.fetch('EXCHANGE_HOST', 'localhost'),
      ENV.fetch('EXCHANGE_PORT', '9000').to_i,
      MarketStreamClient,
      ENV.fetch('INSTRUMENTS', 'BTC/USD,ETH/USD').split(','),
      ->(data) {
        $message_count += data.is_a?(Array) ? data.length : 1
      },
      ->(error) {
        $logger.error "Market stream error: #{error.message}"
      },
      $redis_manager
    )

    # Start REST API in a separate thread
    Thread.new do
      $logger.info "Starting REST API on #{Constants::API_HOST}:#{Constants::API_PORT}"
      MarketStreamAPI.run!
    end

    $logger.info "v2 MarketStream service started successfully"
    $logger.info "  Instruments: #{$client.instrument_ids.join(', ')}"
    $logger.info "  API: http://#{Constants::API_HOST}:#{Constants::API_PORT}"
    $logger.info "  Redis channels: #{Constants::REDIS_PUB_CHANNELS.join(', ')}"
    $logger.info "  PID: #{Process.pid}"
  end
rescue Interrupt
  $logger.info "Service stopped by interrupt. Cleaning up..."
  $redis_manager&.shutdown
rescue StandardError => e
  $logger.error "Fatal error starting service: #{e.message}"
  $logger.error e.backtrace.first(10).join("\n")
  exit 1
end

# ===─ CLI =========================================================================================================

case ARGV.first
when 'start'
  $logger.info "v2 MarketStream v#{V2_VERSION} (#{V2_BUILD})"
  start_service
when 'stop'
  $logger.info "Stop requested. Sending SIGTERM to #{Process.pid}"
  Process.kill('TERM', Process.pid)
when 'restart'
  $logger.info "Restarting... This might not work. It usually crashes on restart."
  $logger.info "The v1 service had the same problem. We tried to fix it but ran out of sprint budget."
  exec("ruby", __FILE__, "start")
when 'status'
  puts "MarketStream v#{V2_VERSION}"
  puts "Status: #{$client&.connected ? 'Connected' : 'Disconnected'}"
  puts "Uptime: #{(Time.now.utc - $start_time).to_i}s"
  puts "Messages: #{$message_count || 0}"
  puts "Redis: #{$redis_manager&.connected ? 'Connected' : 'Disconnected'}"
  puts "Fucks given: 0"
when '--version', '-v'
  puts "MarketStream v#{V2_VERSION} (#{V2_BUILD})"
when '--help', '-h'
  puts "Usage: #{$PROGRAM_NAME} [start|stop|restart|status|--version|--help]"
  puts ""
  puts "  start    Start the market stream service"
  puts "  stop     Stop the market stream service"
  puts "  restart  Restart the market stream service (lol)"
  puts "  status   Show service status"
  puts "  --version, -v  Show version"
  puts "  --help, -h     Show this help"
else
  $stderr.puts "Unknown command: #{ARGV.first}"
  $stderr.puts "Usage: #{$PROGRAM_NAME} [start|stop|restart|status]"
  exit 1
end
