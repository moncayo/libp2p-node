CREATE TABLE IF NOT EXISTS eth_prices (
    id SERIAL PRIMARY KEY,
    price NUMERIC NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL UNIQUE,
    signatures JSONB NOT NULL
);

INSERT INTO eth_prices (price, timestamp, signatures) VALUES (0, NOW(), '{}');
