-- How has spending in a category, or at a payee, changed over time, by year or by month, in one currency?
WITH params AS (SELECT 'Food:Groceries' AS category, CAST(NULL AS VARCHAR) AS payee, 'year' AS grain, DATE '2022-01-01' AS since, current_date AS until, 'CAD' AS currency)
SELECT
    CAST(date_trunc(p.grain, s.date) AS DATE) AS period,
    CASE WHEN CASE p.currency WHEN 'CAD' THEN s.spent_cad WHEN 'USD' THEN s.spent_usd END IS NULL THEN s.currency ELSE p.currency END AS currency,
    CAST(sum(COALESCE(CASE p.currency WHEN 'CAD' THEN s.spent_cad WHEN 'USD' THEN s.spent_usd END, s.spent)) AS DECIMAL(18,2)) AS spent
FROM v_spending s, params p
WHERE s.date BETWEEN p.since AND p.until
    AND (p.category IS NULL
        OR lower(s.category) = lower(p.category)
        OR starts_with(lower(s.category), lower(p.category) || ':'))
    AND (p.payee IS NULL OR lower(s.payee) = lower(p.payee))
GROUP BY 1, 2
ORDER BY 1, 2
