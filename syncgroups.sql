--
-- PostgreSQL database dump
--

-- Dumped from database version 10.8 (Ubuntu 10.8-0ubuntu0.18.04.1)
-- Dumped by pg_dump version 11.5

-- Started on 2019-10-04 12:44:28

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

SET default_with_oids = false;

--
-- TOC entry 200 (class 1259 OID 131268)
-- Name: syncgroups; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.syncgroups (
    id bigint NOT NULL,
    active boolean DEFAULT true NOT NULL,
    direction public.syncdirection DEFAULT 'none'::public.syncdirection NOT NULL,
    tags boolean DEFAULT false NOT NULL
);


ALTER TABLE public.syncgroups OWNER TO postgres;

--
-- TOC entry 2903 (class 0 OID 131268)
-- Dependencies: 200
-- Data for Name: syncgroups; Type: TABLE DATA; Schema: public; Owner: postgres
--

INSERT INTO public.syncgroups VALUES (2250437, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510009, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510088, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510032, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2061689, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2317722, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2180340, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2327162, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2315925, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (704562, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2206003, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1624911, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510037, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1803850, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2068924, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510020, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1512203, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1379562, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1378881, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510034, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2066935, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2171463, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2180978, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1510019, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2171465, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1387750, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2260611, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2061687, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (2174844, false, 'bothlocal', false);
INSERT INTO public.syncgroups VALUES (1385802, true, 'bothlocal', false);


--
-- TOC entry 2779 (class 2606 OID 131273)
-- Name: syncgroups syncgroups_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.syncgroups
    ADD CONSTRAINT syncgroups_pkey PRIMARY KEY (id);


-- Completed on 2019-10-04 12:44:29

--
-- PostgreSQL database dump complete
--

