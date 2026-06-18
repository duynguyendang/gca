---
type: BigQuery Table
title: Orders
description: One row per completed customer order.
tags: [sales, orders]
timestamp: '2026-01-15T00:00:00Z'
---

# Schema

| Column | Type | Description |
|--------|------|-------------|
| order_id | STRING | Unique order identifier |
| customer_id | STRING | FK to [customers](/tables/customers.md) |
| total_usd | NUMERIC | Order total in USD |

Part of the [sales dataset](/datasets/sales.md).

# Citations

- https://example.com/docs/orders
