BEGIN;
INSERT INTO products (sku, name, price, stock, is_active)
VALUES (
        'A-001',
        'Laptop ASUS ROG',
        8500000,
        20,
        TRUE
    ),
    ('A-002', 'Mouse Logitech M331', 150000, 50, TRUE),
    (
        'A-003',
        'Keyboard Mechanical Keychron',
        1250000,
        15,
        TRUE
    ),
    (
        'A-004',
        'Monitor ASUS ROG 144Hz',
        1800000,
        12,
        TRUE
    ),
    ('A-005', 'USB-C Hub UGREEN', 350000, 30, TRUE),
    ('A-006', 'Webcam Logitech ', 950000, 8, TRUE),
    ('A-007', 'Headset Hyper X', 650000, 25, TRUE),
    ('A-008', 'SSD Samsung 1TB', 1450000, 10, TRUE),
    (
        'A-009',
        'RAM Kingston DDR5 16GB',
        950000,
        18,
        TRUE
    ),
    (
        'A-010',
        'Router TP-Link',
        750000,
        14,
        TRUE
    ),
    ('A-011', 'Printer Epson', 2450000, 7, TRUE),
    (
        'A-012',
        'External HDD Seagate 2TB',
        1250000,
        22,
        TRUE
    ) ON CONFLICT (sku) DO NOTHING;
DO $$
DECLARE v_order_id INTEGER;
v_total BIGINT;
BEGIN IF NOT EXISTS (
    SELECT 1
    FROM sales_orders
    WHERE customer = 'Agus'
) THEN IF EXISTS (
    SELECT 1
    FROM (
            VALUES ('A-001', 2),
                ('A-002', 3)
        ) AS item(sku, qty)
        LEFT JOIN products p ON p.sku = item.sku
    WHERE p.id IS NULL
        OR p.stock < item.qty
) THEN RAISE EXCEPTION 'Insufficient stock for Agus order';
END IF;
SELECT SUM(p.price::BIGINT * item.qty) INTO v_total
FROM (
        VALUES ('A-001', 2),
            ('A-002', 3)
    ) AS item(sku, qty)
    JOIN products p ON p.sku = item.sku;
INSERT INTO sales_orders (customer, total)
VALUES ('Agus', v_total::INTEGER)
RETURNING id INTO v_order_id;
INSERT INTO sales_order_items (order_id, sku, qty, unit_price)
SELECT v_order_id,
    item.sku,
    item.qty,
    p.price
FROM (
        VALUES ('A-001', 2),
            ('A-002', 3)
    ) AS item(sku, qty)
    JOIN products p ON p.sku = item.sku;
UPDATE products p
SET stock = p.stock - item.qty
FROM (
        VALUES ('A-001', 2),
            ('A-002', 3)
    ) AS item(sku, qty)
WHERE p.sku = item.sku
    AND p.stock >= item.qty;
END IF;
IF NOT EXISTS (
    SELECT 1
    FROM sales_orders
    WHERE customer = 'Michael'
) THEN IF EXISTS (
    SELECT 1
    FROM (
            VALUES ('A-003', 1),
                ('A-004', 2),
                ('A-006', 1)
        ) AS item(sku, qty)
        LEFT JOIN products p ON p.sku = item.sku
    WHERE p.id IS NULL
        OR p.stock < item.qty
) THEN RAISE EXCEPTION 'Insufficient stock for Michael order';
END IF;
SELECT SUM(p.price::BIGINT * item.qty) INTO v_total
FROM (
        VALUES ('A-003', 1),
            ('A-004', 2),
            ('A-006', 1)
    ) AS item(sku, qty)
    JOIN products p ON p.sku = item.sku;
INSERT INTO sales_orders (customer, total)
VALUES ('Michael', v_total::INTEGER)
RETURNING id INTO v_order_id;
INSERT INTO sales_order_items (order_id, sku, qty, unit_price)
SELECT v_order_id,
    item.sku,
    item.qty,
    p.price
FROM (
        VALUES ('A-003', 1),
            ('A-004', 2),
            ('A-006', 1)
    ) AS item(sku, qty)
    JOIN products p ON p.sku = item.sku;
UPDATE products p
SET stock = p.stock - item.qty
FROM (
        VALUES ('A-003', 1),
            ('A-004', 2),
            ('A-006', 1)
    ) AS item(sku, qty)
WHERE p.sku = item.sku
    AND p.stock >= item.qty;
END IF;
END $$;
COMMIT;