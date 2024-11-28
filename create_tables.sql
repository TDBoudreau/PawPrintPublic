SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

-- ------------------------
-- Create the users table
-- ------------------------
CREATE TABLE public.users (
    id INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    first_name VARCHAR(255) DEFAULT '' NOT NULL,
    last_name VARCHAR(255) DEFAULT '' NOT NULL,
    email VARCHAR(255) NOT NULL,
    password VARCHAR(60) NOT NULL,
    access_level INTEGER DEFAULT 1 NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Create a unique index on the email column
CREATE UNIQUE INDEX users_email_idx ON public.users (email);

-- Insert initial data
-- Temporarily disable identity insert restrictions
SET session_replication_role = 'replica';

COPY public.users (id, first_name, last_name, email, password, access_level, created_at, updated_at) FROM stdin;
1	Timothy	Boudreau	admin@admin.com	$2a$12$Wm8SHtNb7v9oRF6RmPP/c.PHE5tERA6mAfvShxcWJWT7i5nwXg94i	3	2024-11-28 00:00:00	2024-11-28 00:00:00
\.

-- Re-enable identity insert restrictions
SET session_replication_role = 'origin';

-- Adjust the sequence to start from the next available id
SELECT pg_catalog.setval(pg_get_serial_sequence('public.users', 'id'), (SELECT MAX(id) FROM public.users), true);

-- ------------------------
-- Create the files table
-- ------------------------
CREATE TABLE public.files (
    id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    task_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_type TEXT CHECK (file_type IN ('csv', 'xlsx', 'pdf')) NOT NULL,
    file_data BYTEA NOT NULL,
    upload_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
