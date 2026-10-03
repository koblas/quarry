-- How much income came in under each category, in one currency?
WITH params AS (SELECT date_trunc('year', current_date) AS since, current_date AS until, 'CAD' AS currency)
SELECT
    COALESCE(f.category, '(uncategorized)') AS category,
    CASE WHEN CASE p.currency WHEN 'CAD' THEN f.amount_cad ELSE f.amount_usd END IS NULL THEN f.currency ELSE p.currency END AS currency,
    CAST(sum(COALESCE(CASE p.currency WHEN 'CAD' THEN f.amount_cad ELSE f.amount_usd END, f.amount)) AS DECIMAL(18,2)) AS income
FROM v_cash_flow f, params p
WHERE f.flow = 'income'
    AND f.date BETWEEN p.since AND p.until
GROUP BY 1, 2
ORDER BY 1, 2
