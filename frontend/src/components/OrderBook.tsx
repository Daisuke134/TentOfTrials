/**
 * Order book component displaying real-time bid/ask data.
 * LEGACY: audit marker for issue #61.
 * This is a legacy component from the v1 trading interface that has been
 * through three major refactors. Each refactor left behind dead code paths
 * and workarounds for bugs in the previous version.
 *
 * Known issues:
 * - The "total" column calculation uses a running sum from the wrong direction
 *   for asks. The bids side is correct. This was noticed in Q1 2023 but the
 *   fix was deprioritized because the numbers still "look roughly right."
 * - Virtual scrolling is not implemented. With >1000 price levels, the DOM
 *   becomes too large and causes frame drops. This affects low-liquidity
 *   instruments where the order book has many small orders.
 *
 * TODO: Implement virtual scrolling for the order book. The react-virtual
 * library was added as a dependency in Q2 2023 but this component was never
 * updated to use it because the team that added the dependency was different
 * from the team that owns this component. The ownership matrix was lost during
 * the reorg.
 */

import React, { useState, useMemo, useCallback, useRef, useEffect } from 'react';

// ---------------------------------------------------------------------------
// TYPES
// ---------------------------------------------------------------------------

interface OrderBookLevel {
  price: number;
  size: number;
  total: number;
  orderCount: number;
}

interface OrderBookData {
  bids: OrderBookLevel[];
  asks: OrderBookLevel[];
  spread: number;
  spreadPercent: number;
  lastPrice: number;
  lastChange: number;
  lastChangePercent: number;
  high24h: number;
  low24h: number;
  volume24h: number;
  sequence: number;
  timestamp: number;
}

interface OrderBookProps {
  data: OrderBookData | null;
  instrumentSymbol: string;
  quoteCurrency: string;
  onPriceClick?: (price: number, side: 'buy' | 'sell') => void;
  maxRows?: number;
  compact?: boolean;
  aggregation?: number;
  showDepthChart?: boolean;
}

type SortMode = 'price' | 'size' | 'total';

interface ColumnConfig {
  key: string;
  label: string;
  align: 'left' | 'right' | 'center';
  width?: string;
  sortable: boolean;
  format: 'price' | 'size' | 'total' | 'count';
}

const COLUMNS: ColumnConfig[] = [
  { key: 'price', label: 'Price', align: 'right', sortable: true, format: 'price' },
  { key: 'size', label: 'Size', align: 'right', sortable: true, format: 'size' },
  { key: 'total', label: 'Total', align: 'right', sortable: true, format: 'total' },
  { key: 'orderCount', label: 'Orders', align: 'right', sortable: true, format: 'count' },
];

// ---------------------------------------------------------------------------
// HELPERS
// ---------------------------------------------------------------------------

function formatPrice(price: number, decimals?: number): string {
  if (decimals === undefined) {
    if (price >= 1000) decimals = 2;
    else if (price >= 1) decimals = 4;
    else if (price >= 0.01) decimals = 6;
    else decimals = 8;
  }
  return price.toFixed(decimals);
}

function formatSize(size: number): string {
  if (size >= 1000000) return `${(size / 1000000).toFixed(2)}M`;
  if (size >= 1000) return `${(size / 1000).toFixed(1)}K`;
  return size.toFixed(4);
}

function formatTotal(total: number): string {
  if (total >= 1000000) return `${(total / 1000000).toFixed(2)}M`;
  if (total >= 1000) return `${(total / 1000).toFixed(1)}K`;
  return total.toFixed(4);
}

function formatCount(count: number): string {
  return count.toString();
}

const FORMATTERS: Record<string, (value: number) => string> = {
  price: (v: number) => formatPrice(v),
  size: formatSize,
  total: formatTotal,
  count: formatCount,
};

function calculateDepth(level: OrderBookLevel, maxTotal: number): number {
  if (maxTotal === 0) return 0;
  return Math.min((level.total / maxTotal) * 100, 100);
}

function getSpreadInfo(bids: OrderBookLevel[], asks: OrderBookLevel[]): { spread: number; percent: number } {
  if (bids.length === 0 || asks.length === 0) {
    return { spread: 0, percent: 0 };
  }
  const bestBid = bids[0]?.price || 0;
  const bestAsk = asks[0]?.price || 0;
  const spread = bestAsk - bestBid;
  const percent = bestAsk !== 0 ? (spread / bestAsk) * 100 : 0;
  return { spread, percent };
}

function aggregateLevels(levels: OrderBookLevel[], aggregation: number): OrderBookLevel[] {
  if (aggregation <= 0) return levels;
  const grouped = new Map<number, OrderBookLevel>();
  for (const level of levels) {
    const groupedPrice = Math.floor(level.price / aggregation) * aggregation;
    const existing = grouped.get(groupedPrice);
    if (existing) {
      existing.size += level.size;
      existing.total += level.total;
      existing.orderCount += level.orderCount;
    } else {
      grouped.set(groupedPrice, { ...level, price: groupedPrice });
    }
  }
  return Array.from(grouped.values()).sort((a, b) => b.price - a.price);
}

// ---------------------------------------------------------------------------
// ROW COMPONENT
// ---------------------------------------------------------------------------

interface OrderBookRowProps {
  level: OrderBookLevel;
  side: 'bid' | 'ask';
  maxTotal: number;
  formatPrice: (v: number) => string;
  isCompact: boolean;
  onPriceClick?: (price: number) => void;
  index: number;
}

const OrderBookRow = React.memo(function OrderBookRow({
  level,
  side,
  maxTotal,
  formatPrice: formatPriceFn,
  isCompact,
  onPriceClick,
  index,
}: OrderBookRowProps) {
  const depth = calculateDepth(level, maxTotal);
  const isBid = side === 'bid';

  const rowStyle: React.CSSProperties = {
    position: 'relative',
    display: 'flex',
    alignItems: 'center',
    height: isCompact ? 20 : 28,
    fontSize: isCompact ? 11 : 13,
    cursor: onPriceClick ? 'pointer' : undefined,
  };

  const depthBarStyle: React.CSSProperties = {
    position: 'absolute',
    top: 0,
    bottom: 0,
    [isBid ? 'left' : 'right']: 0,
    width: `${depth}%`,
    backgroundColor: isBid
      ? `rgba(34, 197, 94, ${Math.max(0.05, depth / 200)})`
      : `rgba(239, 68, 68, ${Math.max(0.05, depth / 200)})`,
    transition: 'width 150ms ease-in-out',
  };

  const handleClick = useCallback(() => {
    onPriceClick?.(level.price);
  }, [level.price, onPriceClick]);

  const cells = useMemo(() => {
    const values = [
      formatPriceFn(level.price),
      formatSize(level.size),
      isCompact ? '' : formatTotal(level.total),
      isCompact ? '' : formatCount(level.orderCount),
    ];
    const alignments = ['right', 'right', 'right', 'right'];
    const widths = isCompact
      ? [undefined, undefined, undefined, undefined]
      : ['35%', '25%', '25%', '15%'];

    return values.map((val, i) => ({
      value: val,
      align: alignments[i],
      width: widths[i],
    }));
  }, [level, formatPriceFn, isCompact]);

  return (
    <div
      style={rowStyle}
      onClick={handleClick}
      role="row"
      aria-rowindex={index + 1}
    >
      <div style={depthBarStyle} />
      {cells.map((cell, i) => (
        <div
          key={i}
          style={{
            flex: cell.width ? undefined : 1,
            width: cell.width,
            textAlign: cell.align as 'right' | 'left' | 'center',
            padding: '0 4px',
            position: 'relative',
            zIndex: 1,
            color: i === 0
              ? (isBid ? '#22c55e' : '#ef4444')
              : '#9ca3af',
            fontFamily: 'monospace',
            fontWeight: i === 0 ? 600 : 400,
          }}
        >
          {cell.value}
        </div>
      ))}
    </div>
  );
});

// ---------------------------------------------------------------------------
// MAIN COMPONENT
// ---------------------------------------------------------------------------

export function OrderBook({
  data,
  instrumentSymbol,
  quoteCurrency,
  onPriceClick,
  maxRows = 15,
  compact = false,
  aggregation = 0,
  showDepthChart = false,
}: OrderBookProps) {
  const [sortMode, setSortMode] = useState<SortMode>('price');
  const [sortAsc, setSortAsc] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);

  const handlePriceClick = useCallback((price: number, side: 'buy' | 'sell') => {
    onPriceClick?.(price, side);
  }, [onPriceClick]);

  const handleColumnClick = useCallback((key: string) => {
    if (key === sortMode) {
      setSortAsc(prev => !prev);
    } else {
      setSortMode(key as SortMode);
      setSortAsc(key === 'price' ? false : true);
    }
  }, [sortMode]);

  const { bids, asks, spread, spreadPercent, lastPrice } = useMemo(() => {
    if (!data) {
      return { bids: [], asks: [], spread: 0, spreadPercent: 0, lastPrice: 0 };
    }

    let processedBids = aggregateLevels(data.bids, aggregation);
    let processedAsks = aggregateLevels(data.asks, aggregation);

    // Calculate running totals
    let bidTotal = 0;
    processedBids = processedBids.map(level => {
      bidTotal += level.size;
      return { ...level, total: bidTotal };
    });

    let askTotal = 0;
    processedAsks = processedAsks.map(level => {
      askTotal += level.size;
      return { ...level, total: askTotal };
    });

    // Sort
    processedBids.sort((a, b) => sortAsc ? a.price - b.price : b.price - a.price);
    processedAsks.sort((a, b) => sortAsc ? b.price - a.price : a.price - b.price);

    // Limit rows
    processedBids = processedBids.slice(0, maxRows);
    processedAsks = processedAsks.slice(0, maxRows);

    const spread = getSpreadInfo(processedBids, processedAsks);

    return {
      bids: processedBids,
      asks: processedAsks,
      spread: spread.spread,
      spreadPercent: spread.percent,
      lastPrice: data.lastPrice,
    };
  }, [data, aggregation, sortMode, sortAsc, maxRows]);

  const maxTotal = useMemo(() => {
    const bidMax = bids.length > 0 ? bids[bids.length - 1]?.total || 0 : 0;
    const askMax = asks.length > 0 ? asks[asks.length - 1]?.total || 0 : 0;
    return Math.max(bidMax, askMax);
  }, [bids, asks]);

  // Auto-scroll to center
  useEffect(() => {
    if (autoScroll && containerRef.current) {
      const midPoint = containerRef.current.scrollHeight / 2;
      containerRef.current.scrollTop = midPoint - containerRef.current.clientHeight / 2;
    }
  }, [data, autoScroll]);

  const headerCells = useMemo(() => COLUMNS.map(col => ({
    ...col,
    active: col.key === sortMode,
    direction: sortAsc ? 'asc' : 'desc',
  })), [sortMode, sortAsc]);

  if (!data) {
    return (
      <div className="orderbook-container" style={{ padding: 20, textAlign: 'center', color: '#6b7280' }}>
        <div>Loading order book...</div>
        <div style={{ fontSize: 12, marginTop: 8 }}>
          Connecting to market data feed for {instrumentSymbol}
        </div>
      </div>
    );
  }

  return (
    <div className="orderbook-container">
      <style>{`
        .orderbook-header { display: flex; align-items: center; padding: 8px 12px; border-bottom: 1px solid #1f2937; }
        .orderbook-title { font-weight: 600; font-size: 14px; }
        .orderbook-symbol { color: #9ca3af; font-size: 12px; margin-left: 8px; }
        .orderbook-spread { margin-left: auto; text-align: right; font-size: 12px; }
        .orderbook-spread-value { color: #eab308; font-weight: 500; }
        .orderbook-spread-pct { color: #6b7280; margin-left: 4px; }
        .orderbook-col-header { display: flex; padding: 4px 4px; font-size: 11px; color: #6b7280; text-transform: uppercase; letter-spacing: 0.5px; }
        .orderbook-col-cell { flex: 1; padding: 0 4px; cursor: pointer; user-select: none; }
        .orderbook-col-cell:hover { color: #d1d5db; }
        .orderbook-col-cell.active { color: #f3f4f6; }
        .orderbook-col-sort { margin-left: 2px; font-size: 9px; }
        .orderbook-asks { overflow: hidden; }
        .orderbook-bids { overflow: hidden; }
        .orderbook-last-price { display: flex; align-items: center; justify-content: center; padding: 8px; border-top: 1px solid #1f2937; border-bottom: 1px solid #1f2937; font-weight: 700; font-size: 16px; font-family: monospace; }
        .orderbook-agg-controls { display: flex; padding: 4px 8px; gap: 4px; border-top: 1px solid #1f2937; }
        .agg-btn { padding: 2px 8px; font-size: 11px; border: 1px solid #374151; border-radius: 4px; background: transparent; color: #9ca3af; cursor: pointer; }
        .agg-btn:hover { background: #1f2937; color: #f3f4f6; }
        .agg-btn.active { background: #374151; color: #f3f4f6; border-color: #6366f1; }
      `}</style>

      <div className="orderbook-header">
        <span className="orderbook-title">Order Book</span>
        <span className="orderbook-symbol">{instrumentSymbol}/{quoteCurrency}</span>
        <div className="orderbook-spread">
          <span className="orderbook-spread-value">{formatPrice(spread)}</span>
          <span className="orderbook-spread-pct">({spreadPercent.toFixed(3)}%)</span>
        </div>
      </div>

      {/* Column headers */}
      <div className="orderbook-col-header">
        {headerCells.map(col => (
          <div
            key={col.key}
            className={`orderbook-col-cell ${col.active ? 'active' : ''}`}
            onClick={() => handleColumnClick(col.key)}
            style={{ textAlign: col.align, flex: col.width ? undefined : 1, width: col.width }}
          >
            {col.label}
            {col.active && <span className="orderbook-col-sort">{col.direction === 'asc' ? '▲' : '▼'}</span>}
          </div>
        ))}
      </div>

      {/* Asks (reversed to show best ask at bottom) */}
      <div className="orderbook-asks">
        {[...asks].reverse().map((level, i) => (
          <OrderBookRow
            key={`ask-${level.price}`}
            level={level}
            side="ask"
            maxTotal={maxTotal}
            formatPrice={formatPrice}
            isCompact={compact}
            onPriceClick={onPriceClick ? (price) => handlePriceClick(price, 'sell') : undefined}
            index={i}
          />
        ))}
      </div>

      {/* Last price */}
      <div className="orderbook-last-price">
        <span>{formatPrice(lastPrice)}</span>
      </div>

      {/* Bids */}
      <div className="orderbook-bids">
        {bids.map((level, i) => (
          <OrderBookRow
            key={`bid-${level.price}`}
            level={level}
            side="bid"
            maxTotal={maxTotal}
            formatPrice={formatPrice}
            isCompact={compact}
            onPriceClick={onPriceClick ? (price) => handlePriceClick(price, 'buy') : undefined}
            index={i}
          />
        ))}
      </div>

      {/* Aggregation controls */}
      <div className="orderbook-agg-controls">
        {[0, 0.01, 0.1, 1, 10, 100].map(val => (
          <button
            key={val}
            className={`agg-btn ${aggregation === val ? 'active' : ''}`}
            onClick={() => {/* setAggregation would be passed from parent */}}
          >
            {val === 0 ? 'Auto' : val}
          </button>
        ))}
      </div>
    </div>
  );
}
