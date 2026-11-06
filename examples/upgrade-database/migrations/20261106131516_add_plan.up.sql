-- momo:generated

ALTER TABLE "customers" ADD COLUMN "plan" varchar(20) DEFAULT 'free' NOT NULL;

CREATE INDEX "customers_plan_idx" ON "customers" ("plan");
