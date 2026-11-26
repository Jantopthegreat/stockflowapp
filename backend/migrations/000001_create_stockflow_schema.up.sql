-- Table: Products
CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    sku TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    price INTEGER NOT NULL CHECK (price >= 0),
    stock INTEGER NOT NULL DEFAULT 0 CHECK (stock >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT products_sku_not_blank CHECK (LENGTH(BTRIM(sku)) > 0),
    CONSTRAINT products_name_not_blank CHECK (LENGTH(BTRIM(name)) > 0)
);
-- Table : Stock Movements
CREATE TABLE stock_movements (
    id SERIAL PRIMARY KEY,
    product_id INTEGER NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('in', 'out')),
    qty INTEGER NOT NULL CHECK (qty > 0),
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT stock_movements_product_fk FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE RESTRICT
);
-- Table : Sales Orders
CREATE TABLE sales_orders (
    id SERIAL PRIMARY KEY,
    customer TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'CREATED',
    total INTEGER NOT NULL CHECK (total >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cancelled_at TIMESTAMPTZ,
    CONSTRAINT sales_orders_customer_not_blank CHECK (LENGTH(BTRIM(customer)) > 0),
    CONSTRAINT sales_orders_status_check CHECK (status IN ('CREATED', 'CANCELLED')),
    CONSTRAINT sales_orders_cancel_consistency CHECK (
        (
            status = 'CREATED'
            AND cancelled_at IS NULL
        )
        OR (
            status = 'CANCELLED'
            AND cancelled_at IS NOT NULL
        )
    )
);
-- Table : Sales Order Items
CREATE TABLE sales_order_items (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL,
    sku TEXT NOT NULL,
    qty INTEGER NOT NULL CHECK (qty > 0),
    unit_price INTEGER NOT NULL CHECK (unit_price >= 0),
    CONSTRAINT sales_order_items_order_fk FOREIGN KEY (order_id) REFERENCES sales_orders(id) ON DELETE RESTRICT,
    CONSTRAINT sales_order_items_product_fk FOREIGN KEY (sku) REFERENCES products(sku) ON DELETE RESTRICT,
    CONSTRAINT sales_order_items_order_sku_unique UNIQUE (order_id, sku)
);
-- Index
CREATE INDEX idx_stock_movements_product_created ON stock_movements (product_id, created_at DESC, id DESC);
CREATE INDEX idx_sales_orders_created ON sales_orders (created_at DESC, id DESC);
CREATE INDEX idx_sales_orders_status ON sales_orders (status, created_at DESC, id DESC);
CREATE INDEX idx_sales_order_items_order ON sales_order_items (order_id);
CREATE INDEX idx_sales_order_items_sku ON sales_order_items (sku);