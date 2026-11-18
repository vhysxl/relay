# Relay • Distributed Workflow & Task Orchestration Engine

Relay adalah distributed workflow engine dan task orchestrator berkinerja tinggi berbasis Go dan MySQL. Relay dirancang untuk mengeksekusi Directed Acyclic Graph (DAG) workflows dengan dukungan Saga pattern (kompensasi otomatis saat gagal), exponential retry backoff, distributed task locking (`SELECT ... FOR UPDATE SKIP LOCKED`), lease heartbeat, dan web observability dashboard.

> **Catatan:** Server Relay diasumsikan berjalan pada host lokal di port `8000` (`http://localhost:8000`).

Backend Go saat ini bertindak sebagai task manager dan workflow engine. Eksekusi bisnis dijalankan oleh **external worker** melalui HTTP long polling; backend tidak menjalankan executor HTTP/RabbitMQ lokal. Panduan pembuatan **`relay-worker-node`** tersedia di [Bagian 4](#4-relay-worker-node--panduan-implementasi).

---

## Pengujian backend

Jalankan `go vet ./...` dan `go test -race -timeout 5m ./...`.
Untuk menyertakan tes integrasi, set `TEST_DB_DSN` ke server **MySQL 8.4 khusus tes**
(contoh `root:test_password@tcp(127.0.0.1:3306)/?parseTime=true&clientFoundRows=true`).
Akun tersebut memerlukan izin CREATE/DROP DATABASE. Setiap tes membuat schema
`relay_test_<uuid>`, menerapkan seluruh migrasi `.up.sql` secara berurutan, lalu
menghapus hanya schema tersebut. Tanpa DSN tes integrasi dilewati; jika DSN sudah
diatur, koneksi atau migrasi yang gagal membuat tes gagal. Job CI `integration`
menyediakan server tersebut dan menjalankan tes dengan race detector.

## Daftar Isi

1. [Workflow & Step Definition Guide](#1-workflow--step-definition-guide)
   - [1.1 Struktur Workflow Definition](#11-struktur-workflow-definition)
   - [1.2 Struktur Step Definition](#12-struktur-step-definition)
   - [1.3 Konfigurasi Tipe Step (HTTP, Message & Activity)](#13-konfigurasi-tipe-step-http-message--activity)
   - [1.4 Dynamic Context & Parameter Interpolation](#14-dynamic-context--parameter-interpolation-data-dinamis-user)
   - [1.5 Aturan Validasi DAG](#15-aturan-validasi-dag)
   - [1.6 Saga Pattern & Definisi Kompensasi](#16-saga-pattern--definisi-kompensasi)
   - [1.7 Contoh Lengkap Workflow Definition (E-Commerce Saga)](#17-contoh-lengkap-workflow-definition-e-commerce-saga)
   - [1.8 Conditional Step Execution (when)](#18-conditional-step-execution-when)
2. [Workflow Calling Guide](#2-workflow-calling-guide)
   - [2.1 Ringkasan REST API Endpoints](#21-ringkasan-rest-api-endpoints)
   - [2.2 Mendaftarkan Workflow Baru (`POST /workflows`)](#22-mendaftarkan-workflow-baru-post-workflows)
   - [2.3 Menjalankan Workflow (`POST /workflows/{name}/execute`)](#23-menjalankan-workflow-post-workflowsnameexecute)
   - [2.4 Memeriksa Status Eksekusi (`GET /executions/{id}`)](#24-memeriksa-status-eksekusi-get-executionsid)
   - [2.5 Contoh Integrasi Client (cURL, TypeScript/NestJS, Go)](#25-contoh-integrasi-client)
3. [Mekanisme Arsitektur Relay & Diagram Pendukung](#3-mekanisme-arsitektur-relay--diagram-pendukung)
   - [3.1 Arsitektur Sistem & Komponen Inti](#31-arsitektur-sistem--komponen-inti)
   - [3.2 Diagram Arsitektur Keseluruhan](#32-diagram-arsitektur-keseluruhan)
   - [3.3 Diagram Alur Eksekusi & Siklus Hidup Worker](#33-diagram-alur-eksekusi--siklus-hidup-worker)
   - [3.4 Diagram State Machine (Workflow & Step Lifecycle)](#34-diagram-state-machine-workflow--step-lifecycle)
   - [3.5 Detail Mekanisme Kunci Engine](#35-detail-mekanisme-kunci-engine)
4. [relay-worker-node — Panduan Implementasi](#4-relay-worker-node--panduan-implementasi)
   - [4.1 Scope dan Struktur Project](#41-scope-dan-struktur-project)
   - [4.2 Konfigurasi Backend dan Worker](#42-konfigurasi-backend-dan-worker)
   - [4.3 Kontrak HTTP Worker API](#43-kontrak-http-worker-api)
   - [4.4 Executor dan Activity Registry](#44-executor-dan-activity-registry)
   - [4.5 Lifecycle Worker dan Penanganan Kegagalan](#45-lifecycle-worker-dan-penanganan-kegagalan)
   - [4.6 Contoh HTTP Client Node.js](#46-contoh-http-client-nodejs)
   - [4.7 Smoke Test Integrasi](#47-smoke-test-integrasi)
   - [4.8 Kriteria Penerimaan](#48-kriteria-penerimaan)

---

## 1. Workflow & Step Definition Guide

Workflow di Relay mendefinisikan sebuah grafik alur kerja terarah (DAG) yang terdiri atas satu atau lebih **Step**.

### 1.1 Struktur Workflow Definition

| Field | Tipe | Wajib | Deskripsi |
| :--- | :--- | :--- | :--- |
| `name` | `string` | **Ya** | Nama pengenal alur kerja (contoh: `order.checkout_saga_v1`). |
| `version` | `int` | **Ya** | Nomor versi integer (kombinasi `name` + `version` bersifat unik di database). |
| `context` | `object` | Tidak | Metadata atau initial state bawaan workflow dalam format JSON. |
| `steps` | `array` | **Ya** | Daftar step/node yang menyusun alur kerja (minimal 1 step). |

### 1.2 Struktur Step Definition

Setiap elemen dalam array `steps` memiliki spesifikasi sebagai berikut:

| Field | Tipe | Wajib | Default | Deskripsi |
| :--- | :--- | :--- | :--- | :--- |
| `id` | `string` | **Ya** | - | Pengenal unik step di dalam workflow (contoh: `step-charge-card`). |
| `name` | `string` | Tidak | - | Nama deskriptif untuk ditampilkan di log/dashboard. |
| `type` | `string` | **Ya** | - | Tipe pekerjaan external worker: `"http"`, `"message"`, atau `"activity"`. |
| `config` | `object` | **Ya** | - | Konfigurasi spesifik runner sesuai tipe yang dipilih (lihat Bagian 1.3). |
| `depends_on` | `string[]` | Tidak | `[]` | ID step lain yang harus selesai berstatus `COMPLETED` atau `SKIPPED` sebelum step ini dijalankan. Jika kosong, step dapat langsung dijalankan saat workflow dimulai. |
| `when` | `object` | Tidak | `null` | Kondisi opsional sebelum step dieksekusi. Jika bernilai `false`, step berstatus `SKIPPED` dan step downstream tetap berjalan (lihat Bagian 1.8). |
| `timeout` | `int` | Tidak | `0` (gunakan default backend) | Batas waktu attempt dalam detik. Nilai `0` memakai `DEFAULT_TIMEOUT` (default `15s`), bukan unlimited. |
| `retry` | `object` | Tidak | `null` | Kebijakan percobaan ulang jika step mengalami error eksekusi. |
| `compensation`| `object` | Tidak | `null` | Definisi aksi rollback jika workflow gagal di step downstream (Saga pattern). |
| `irreversible`| `boolean`| Tidak | `false` | Menandai bahwa step tidak dapat di-rollback. Jika `true`, `compensation` dilarang diset. |

#### Struktur Object `retry`
```json
{
  "max_attempts": 3,
  "backoff": 2,
  "multiplier": 2.0,
  "max_backoff": 60
}
```
- `max_attempts` (*int*): Jumlah maksimal eksekusi percobaan (termasuk attempt pertama).
- `backoff` (*int*): Delay awal sebelum retry berikutnya (dalam detik).
- `multiplier` (*float*): Faktor pengali interval retry (exponential backoff). Formula: `backoff * (multiplier ^ (attempt - 1))`.
- `max_backoff` (*int*): Batas maksimum jeda retry (dalam detik).

#### Struktur Object `compensation`
```json
{
  "type": "http",
  "config": {
    "queue": "orders",
    "method": "POST",
    "url": "https://payment.internal/refund",
    "body": { "reason": "saga_rollback" }
  },
  "timeout": 15,
  "retry": {
    "max_attempts": 3,
    "backoff": 1
  }
}
```

---

### 1.3 Konfigurasi Tipe Step (HTTP, Message & Activity)

Seluruh tipe dieksekusi oleh external worker. Field `config.queue` menentukan task queue Relay, bukan nama antrean RabbitMQ. `config.queue` wajib diisi untuk semua tipe step dan kompensasi; tidak ada default queue. Nama queue/activity menggunakan karakter `A-Z`, `a-z`, `0-9`, `.`, `_`, atau `-`, dengan panjang 1–128 karakter.

#### Tipe 1: HTTP Step (`type: "http"`)
Digunakan untuk memanggil REST API atau web service eksternal.

```json
{
  "id": "step-charge-card",
  "name": "Charge Credit Card",
  "type": "http",
  "config": {
    "queue": "orders",
    "method": "POST",
    "url": "https://api.payment.com/v1/charge",
    "headers": {
      "Authorization": "Bearer sec_test_xyz",
      "Content-Type": "application/json"
    },
    "body": {
      "amount": 150000,
      "currency": "IDR"
    }
  }
}
```

#### Tipe 2: RabbitMQ Message Step (`type: "message"`)
Digunakan untuk menerbitkan event asinkron ke antrean/exchange message broker RabbitMQ.

```json
{
  "id": "step-notify-warehouse",
  "name": "Notify Warehouse System",
  "type": "message",
  "config": {
    "queue": "orders",
    "exchange": "logistics_topic",
    "routing_key": "warehouse.orders.prepare",
    "content_type": "application/json",
    "headers": {
      "x-priority": 1
    },
    "payload": {
      "order_id": "ORD-99812",
      "sku": "VCR-50K",
      "quantity": 1
    }
  }
}
```

---

#### Tipe 3: Activity Step (`type: "activity"`)

Digunakan untuk menjalankan handler bisnis yang terdaftar di external worker. `config.queue` dan `config.activity` wajib diisi.

```json
{
  "id": "reserve-stock",
  "type": "activity",
  "config": {
    "queue": "orders",
    "activity": "inventory.reserve",
    "input": {
      "order_id": "{{context.order_id}}"
    }
  },
  "timeout": 60
}
```

### 1.4 Dynamic Context & Parameter Interpolation (Data Dinamis User)

Dalam sistem orkestrator seperti Relay, **Workflow Definition adalah Template (Blueprint)**. Kita **tidak pernah meng-hardcode** data transaksi dinamis (seperti `cart_id`, `user_id`, atau `amount`) di dalam definisi alur.

Sebagai gantinya, gunakan placeholder template:
- `{{context.field}}` atau `{{field}}`
- `{{context.nested.field}}`

#### Contoh Step Definition dengan Parameter Dinamis:
```json
{
  "id": "validate-cart",
  "name": "Validate Cart Items",
  "type": "http",
  "config": {
    "queue": "orders",
    "method": "POST",
    "url": "http://cart-service.internal/carts/{{context.cart_id}}/validate",
    "headers": {
      "X-User-ID": "{{context.user_id}}",
      "Content-Type": "application/json"
    },
    "body": {
      "cart_id": "{{context.cart_id}}",
      "user_id": "{{context.user_id}}",
      "payment_channel": "{{context.payment_channel}}"
    }
  },
  "timeout": 5
}
```

#### Cara Client Mengisi Data Dinamis Saat Runtime:
Saat client (misal NestJS Checkout Service) men-trigger eksekusi alur, sertakan object `context`:
```http
POST http://localhost:8000/workflows/ecommerce.checkout_saga/execute
Content-Type: application/json
Idempotency-Key: checkout:CART-USER-9921

{
  "context": {
    "cart_id": "CART-USER-9921",
    "user_id": "USR-10882",
    "payment_channel": "BCA_VA"
  }
}
```
Relay secara otomatis me-resolve dan menggantikan seluruh placeholder `{{context.cart_id}}` dan `{{context.user_id}}` di dalam URL, headers, dan body sebelum dieksekusi oleh worker!

---

### 1.5 Aturan Validasi DAG

Relay melakukan validasi integritas struktur grafik sebelum workflow disimpan ke database:
1. **Acyclic Check**: Tidak boleh ada dependency melingkar (*cyclic dependency*), baik langsung (`A -> B -> A`) maupun tidak langsung (`A -> B -> C -> A`).
2. **No Self-Dependency**: Step tidak boleh bergantung pada dirinya sendiri (`depends_on: ["self"]`).
3. **Missing Dependency Detection**: Step tidak boleh mencantumkan `depends_on` ID yang tidak terdaftar di dalam array `steps`.
4. **Unique Step ID**: Setiap step dalam satu workflow wajib memiliki `id` yang unik.
5. **Config Schema**: Field `config` harus sesuai dengan tipe eksekutor (misal: HTTP wajib memiliki `method` dan URL valid; RabbitMQ wajib memiliki `exchange` atau `routing_key`).
6. **Irreversible vs Compensation**: Jika `irreversible: true`, step **dilarang** memiliki `compensation`.
7. **Condition Tree & Anchor Validation (`when`)**: Jika step mendefinisikan kondisi `when`, pohon kondisi divalidasi tidak melebihi batas `CONDITION_MAX_DEPTH` dan `CONDITION_MAX_NODES`. Anchor output (`steps.<id>.output...`) wajib merujuk ke step ancestor dalam DAG, dan anchor context dilarang bertabrakan dengan ID root step apa pun.

---

### 1.6 Saga Pattern & Definisi Kompensasi

Jika sebuah alur transaksi bisnis lintas-service mengalami kegagalan pada step di tengah jalan, Relay akan menjalankan mekanisme **Saga Compensation**:
- Hanya step-step yang **telah berhasil (`COMPLETED`)** sebelum terjadinya kegagalan yang akan dieksekusi kompensasinya.
- Step kompensasi dieksekusi dengan prioritas darurat (`is_compensation = TRUE`).
- Step downstream yang belum sempat dijalankan akan otomatis dialihkan ke status `CANCELLED`.
- Jika seluruh step kompensasi sukses, status workflow ditutup sebagai `COMPENSATED`.

---

### 1.7 Contoh Lengkap Workflow Definition (E-Commerce Saga)

Berikut adalah contoh lengkap definisi alur **Checkout Saga**:
1. Step 1: Validasi Keranjang (`validate-cart`).
2. Step 2: Kunci Stok Produk (`reserve-inventory`) setelah keranjang valid; dilengkapi rollback pembatalan stok.
3. Step 3: Potong Pembayaran (`charge-payment`) setelah stok dikunci; dilengkapi rollback refund.
4. Step 4: Terbitkan Voucher (`issue-voucher`) setelah pembayaran sukses.

```json
{
  "name": "ecommerce.checkout_saga",
  "version": 1,
  "context": {
    "domain": "checkout"
  },
  "steps": [
    {
      "id": "validate-cart",
      "name": "Validate Cart Items",
      "type": "http",
      "config": {
        "queue": "orders",
        "method": "POST",
        "url": "http://cart-service.internal/validate",
        "body": { "cart_id": "CART-12345" }
      },
      "timeout": 5
    },
    {
      "id": "reserve-inventory",
      "name": "Reserve Product Stock",
      "type": "http",
      "depends_on": ["validate-cart"],
      "config": {
        "queue": "orders",
        "method": "POST",
        "url": "http://inventory.internal/reserve",
        "body": { "item_id": "VOUCHER-100K", "qty": 1 }
      },
      "retry": {
        "max_attempts": 2,
        "backoff": 1
      },
      "compensation": {
        "type": "http",
        "config": {
          "queue": "orders",
          "method": "POST",
          "url": "http://inventory.internal/release",
          "body": { "item_id": "VOUCHER-100K", "qty": 1 }
        }
      }
    },
    {
      "id": "charge-payment",
      "name": "Charge Customer Payment",
      "type": "http",
      "depends_on": ["reserve-inventory"],
      "config": {
        "queue": "orders",
        "method": "POST",
        "url": "http://payment.internal/charge",
        "body": { "amount": 100000, "method": "EWALLET" }
      },
      "retry": {
        "max_attempts": 3,
        "backoff": 2,
        "multiplier": 2.0
      },
      "compensation": {
        "type": "http",
        "config": {
          "queue": "orders",
          "method": "POST",
          "url": "http://payment.internal/refund",
          "body": { "reason": "saga_failure" }
        }
      }
    },
    {
      "id": "issue-voucher",
      "name": "Issue Electronic Voucher",
      "type": "http",
      "depends_on": ["charge-payment"],
      "config": {
        "queue": "orders",
        "method": "POST",
        "url": "http://voucher-engine.internal/issue",
        "body": { "voucher_code": "DISC100K" }
      },
      "retry": {
        "max_attempts": 3,
        "backoff": 1
      }
    }
  ]
}
```

---

### 1.8 Conditional Step Execution (`when`)

Relay mendukung percabangan deklaratif dengan menambahkan properti `when` opsional pada step HTTP maupun Message normal. Kondisi membaca satu anchor eksplisit per perbandingan:
- **Anchor Context**: `"ref": "user.tier"` membaca langsung dari root payload `context` eksekusi.
- **Anchor Output Step**: `"ref": "steps.<id>.output.path.to.field"` membaca output dari step ancestor yang telah selesai (`COMPLETED`).

> **Penting**: Engine tidak melakukan pencarian sumber alternatif atau interpolasi ekspresi pada `when`. Anchor output harus memakai format kanonis `steps.<id>.output...`, dan validator menolak jika step tersebut bukan ancestor dalam DAG atau jika anchor context bertabrakan dengan ID root step.

#### Bentuk Kondisi

1. **Leaf Node**: Membandingkan anchor dengan operator dan nilai target.
```json
{
  "ref": "steps.check_user.output.tier",
  "op": "eq",
  "value": "diamond"
}
```
2. **Logical Group**: Menggabungkan kumpulan kondisi secara terurut (`all` untuk AND, `any` untuk OR).
```json
{
  "all": [
    { "ref": "user.is_active", "op": "eq", "value": true },
    { "ref": "user.age", "op": "gte", "value": 18 }
  ]
}
```
Setiap node kondisi hanya boleh memilih satu bentuk: berupa leaf `{ref, op, value}` atau grup `{"all": [...]}` / `{"any": [...]}`. Grup wajib berisi minimal satu kondisi anak.

#### Operator yang Didukung

| Operator | Deskripsi | Aturan Tipe Nilai |
| :--- | :--- | :--- |
| `eq` | Sama dengan | Scalar JSON (string, number, boolean) atau `null`. |
| `ne` | Tidak sama dengan | Scalar JSON (string, number, boolean) atau `null`. |
| `exists` | Memeriksa apakah path/properti tersedia | **Dilarang** menyertakan `value`. Mengembalikan `true` jika path ditemukan (meski nilainya `null`). |
| `gt` | Lebih besar dari | Angka numerik. |
| `gte` | Lebih besar atau sama dengan | Angka numerik. |
| `lt` | Lebih kecil dari | Angka numerik. |
| `lte` | Lebih kecil atau sama dengan | Angka numerik. |
| `in` | Membership dalam himpunan | Array scalar seragam (misal: array string atau array number). |

#### Aturan Presisi Angka & Tipe Data
- Relay membandingkan angka menggunakan representasi numerik presisi tinggi (`big.Rat` / `json.Number`). Angka di atas $2^{53}$ dibandingkan secara akurat tanpa kehilangan presisi float64.
- Tidak ada konversi implisit (*no coercion*); string `"100"` tidak akan cocok dengan angka `100`.
- Operator urutan (`gt`, `gte`, `lt`, `lte`) hanya menerima tipe numerik pada `actual` dan `value`.

#### Missing Path vs JSON `null` & Short-Circuit Guard
- Path yang tidak ditemukan (*missing path*) dibedakan dari nilai JSON literal `null`. Operator selain `exists` akan mengembalikan evaluasi `error` jika target path tidak ditemukan.
- Evaluasi pada `all` dan `any` dievaluasi terurut dari kiri ke kanan dan melakukan **short-circuit** begitu hasilnya pasti.
- Pola guard dengan `exists`: Anda dapat memeriksa keberadaan properti opsional sebelum membaca nilainya:
```json
{
  "when": {
    "all": [
      { "ref": "steps.enrich_user.output.profile", "op": "exists" },
      { "ref": "steps.enrich_user.output.profile.score", "op": "gt", "value": 80 }
    ]
  }
}
```
Jika `enrich_user.output.profile` tidak ada, grup `all` langsung berhenti (*short-circuit*) pada kondisi pertama tanpa mencoba membaca path kedua, sehingga terhindar dari error missing path.

#### Siklus Hidup & Perilaku Runtime
Evaluasi kondisi dilakukan saat worker meminta task (`ClaimTask`), setelah seluruh dependensi terpenuhi dan sebelum resolusi config payload:
- **True**: Transisi ke status `RUNNING`, attempt bertambah, audit evaluasi dicatat, dan executor dijalankan normal.
- **False (`SKIPPED`)**: Step bertransisi ke status `SKIPPED` dengan attempt tetap `0`. Tidak ada lease worker, output, `executed_at`, atau pemanggilan executor. Downstream dependencies yang bergantung pada step ini tetap terpenuhi (`COMPLETED` maupun `SKIPPED`).
- **Error / Missing Anchor**: Evaluasi gagal (misal: path missing pada operator selain `exists` atau tipe data tidak kompatibel). Step bertransisi ke `FAILED` dengan attempt `0`, memicu alur kegagalan workflow dan rollback/kompensasi.
- **Kompensasi Step Skipped**: Kompensasi milik step yang `SKIPPED` dialihkan ke `CANCELLED`. Traversal rollback tetap melewati step skipped untuk menjaga urutan kompensasi ancestor sebelumnya.
- **Audit Evaluasi**: Audit evaluasi disimpan pada kolom `condition_evaluation` di `step_executions` dan dapat dipantau langsung melalui dashboard execution detail.

#### Konfigurasi Batas Pohon Kondisi (Environment Variables)

| Variabel Lingkungan | Default | Keterangan |
| :--- | :---: | :--- |
| `CONDITION_MAX_DEPTH` | `8` | Batas kedalaman rekursi maksimal pohon kondisi `when`. |
| `CONDITION_MAX_NODES` | `64` | Batas jumlah node maksimal (leaf + grup) dalam satu kondisi step. |

Keduanya wajib bilangan bulat positif dan divalidasi pada saat startup engine sebelum koneksi database dibuka.

---

## 2. Workflow Calling Guide

Relay mengekspos REST API standar untuk mendaftarkan definisi alur kerja, men-trigger eksekusi, serta memantau status eksekusi.

### 2.1 Ringkasan REST API Endpoints

| Method | Endpoint | Deskripsi |
| :--- | :--- | :--- |
| `POST` | `/workflows` | Mendaftarkan workflow definition baru beserta steps DAG. |
| `GET` | `/workflows` | Mengambil ringkasan versi terbaru setiap workflow: metadata dan `step_count`, tanpa context atau steps/config. Mendukung `limit` (default 50, maksimum 100) dan `offset`. |
| `GET` | `/workflows/{name}` | Mengambil detail workflow definition spesifik berdasarkan nama (versi terbaru). |
| `POST` | `/workflows/{name}/execute` | Memulai/men-trigger instance eksekusi baru dari workflow definition berdasarkan nama. |
| `GET` | `/executions` | Mengambil seluruh riwayat tiket eksekusi beserta detail step-nya. |
| `GET` | `/executions/{id}` | Mengambil data status dan output dari sebuah tiket eksekusi spesifik. |
| `GET` | `/dashboard` / `GET /` | Mengakses Web Observability Dashboard interaktif. |
| `GET` | `/history` | Mengakses halaman tabular riwayat Execution History. |

Contoh response `GET /workflows?limit=50&offset=0`:

```json
[
  {
    "id": "710d3e9f-0e4f-4f1e-9189-02c731e0672c",
    "name": "ecommerce.checkout_saga",
    "version": 1,
    "created_at": "2026-09-23T23:30:00Z",
    "updated_at": "2026-09-23T23:30:00Z",
    "step_count": 4
  }
]
```

Untuk mengambil context, konfigurasi, dan seluruh step DAG, panggil endpoint detail `GET /workflows/{name}`. List kosong tetap dikembalikan sebagai `[]`; `step_count` tetap tersedia ketika nilainya nol.

---

### 2.2 Mendaftarkan Workflow Baru (`POST /workflows`)

Endpoint ini dipanggil sekali saat inisialisasi aplikasi (atau pipeline CI/CD) untuk mendaftarkan struktur alur kerja.

- **Request:**
  ```http
  POST http://localhost:8000/workflows
  Content-Type: application/json
  ```
  *(Body JSON seperti pada Bagian 1.7)*

- **Response:** `201 Created` (cuplikan metadata; response lengkap juga berisi `steps`)
  ```json
  {
    "id": "710d3e9f-0e4f-4f1e-9189-02c731e0672c",
    "name": "ecommerce.checkout_saga",
    "version": 1,
    "created_at": "2026-09-23T23:30:00Z",
    "updated_at": "2026-09-23T23:30:00Z"
  }
  ```

---

### 2.3 Menjalankan Workflow (`POST /workflows/{name}/execute`)

Dipanggil oleh client service (misal NestJS Checkout Service) saat ada permintaan transaksi yang ingin didelegasikan ke Relay.

> **Catatan:** Workflow diidentifikasi **hanya melalui nama**. Parameter `{name}` pada URL adalah nama workflow. UUID diperlakukan sebagai string nama biasa; jika tidak ada workflow bernama UUID tersebut, endpoint mengembalikan `404 Not Found`.

#### Ketentuan Idempotency-Key
- Header `Idempotency-Key` bersifat **wajib** (`1–128` byte ASCII printable tanpa whitespace).
- Key berlaku dalam scope **nama workflow logis** lintas versi.
- Response mengembalikan status `201 Created` dengan header `Idempotency-Replayed: false` untuk eksekusi baru dan `Idempotency-Replayed: true` untuk replay permintaan yang identik.
- Jika key yang sama dikirimkan dengan input context berbeda, endpoint mengembalikan `409 Conflict`. Replay dengan key dan input context yang sama pada nama workflow yang sama akan selalu mengembalikan receipt awal meskipun versi baru workflow telah dibuat.
- Request body memakai envelope `{"context": {...}, "workflow_version": 1}`. Field top-level lain ditolak dengan `400 Bad Request`. `context` dan `workflow_version` opsional; body kosong, `null`, `{}`, dan context null memakai context kosong. Versi eksplisit wajib integer positif.
- Untuk memilih versi tertentu, gunakan wrapper `{"workflow_version": 2, "context": {...}}`. Jika versi tidak ditentukan, execution baru memakai versi terbaru. Replay dengan versi eksplisit yang berbeda dari target awal menghasilkan `409 Conflict`.

- **Request (Memanggil langsung dengan Nama Workflow):**
  ```http
  POST http://localhost:8000/workflows/ecommerce.checkout_saga/execute
  Content-Type: application/json
  Idempotency-Key: checkout:ORD-20260923-001

  {
    "context": {
      "cart_id": "CART-USER-9921",
      "user_id": "USR-8891",
      "payment_method": "BCA_VA"
    }
  }
  ```

- **Response:** `201 Created`
  ```http
  HTTP/1.1 201 Created
  Content-Type: application/json
  Idempotency-Replayed: false

  {
    "id": "afd70011-56d5-45f6-a590-bf514bf2091d",
    "workflow_id": "710d3e9f-0e4f-4f1e-9189-02c731e0672c",
    "status": "RUNNING",
    "context": {
      "order_id": "ORD-20260923-001",
      "user_id": "USR-8891",
      "payment_method": "BCA_VA"
    },
    "total_steps": 4,
    "created_at": "2026-09-23T23:30:15Z"
  }
  ```

---

### 2.4 Memeriksa Status Eksekusi (`GET /executions/{id}`)

Dipanggil untuk polling atau mendapatkan status akhir tiket eksekusi.

Parameter `{id}` adalah **Execution Ticket UUID** dari response execute. Endpoint ini tidak melakukan lookup berdasarkan nama workflow.

- **Request:**
  ```http
  GET http://localhost:8000/executions/afd70011-56d5-45f6-a590-bf514bf2091d
  ```

- **Response:** `200 OK`
  ```json
  {
    "id": "afd70011-56d5-45f6-a590-bf514bf2091d",
    "workflow_id": "710d3e9f-0e4f-4f1e-9189-02c731e0672c",
    "workflow_name": "ecommerce.checkout_saga",
    "status": "COMPLETED",
    "created_at": "2026-09-23T23:30:15Z",
    "updated_at": "2026-09-23T23:30:18Z",
    "steps": [
      {
        "id": "2f6928f7-31cd-47bb-a5b1-980c1ff9914d",
        "step_id": "validate-cart",
        "name": "Validate Cart Items",
        "type": "http",
        "status": "COMPLETED",
        "output": { "valid": true }
      },
      {
        "id": "5dfaaf17-a8d0-4c05-837b-ebeff8726f5f",
        "step_id": "reserve-inventory",
        "name": "Reserve Product Stock",
        "type": "http",
        "status": "COMPLETED",
        "output": { "reservation_token": "RES-881" }
      }
    ]
  }
  ```

---

### 2.5 Contoh Integrasi Client

#### A. Menggunakan cURL
```bash
# 1. Trigger eksekusi workflow dengan Idempotency-Key wajib
EXECUTION_ID=$(curl -s -X POST http://localhost:8000/workflows/ecommerce.checkout_saga/execute \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: checkout:ORD-1001" \
  -d '{"context":{"order_id":"ORD-1001"}}' | jq -r '.id')

# 2. Cek status eksekusi
curl -s http://localhost:8000/executions/$EXECUTION_ID | jq .
```

#### B. Menggunakan TypeScript / NestJS
```typescript
import axios from 'axios';

export class RelayClient {
  private readonly baseUrl = 'http://localhost:8000';

  async triggerWorkflow(workflowId: string, idempotencyKey: string, context: Record<string, any>) {
    const response = await axios.post(
      `${this.baseUrl}/workflows/${workflowId}/execute`,
      { context },
      {
        headers: {
          'Idempotency-Key': idempotencyKey,
        },
      }
    );
    return response.data; // { id: "execution-uuid", status: "RUNNING", ... }
  }

  async getExecutionStatus(executionId: string) {
    const response = await axios.get(`${this.baseUrl}/executions/${executionId}`);
    return response.data;
  }
}
```

#### C. Menggunakan Go
```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func TriggerWorkflow(workflowID string, idempotencyKey string, payload map[string]interface{}) (string, error) {
	body, _ := json.Marshal(map[string]interface{}{"context": payload})
	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("http://localhost:8000/workflows/%s/execute", workflowID),
		bytes.NewBuffer(body),
	)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}
```

---

## 3. Mekanisme Arsitektur Relay & Diagram Pendukung

### 3.1 Arsitektur Sistem & Komponen Inti

| Komponen | Lokasi | Tanggung jawab |
| :--- | :--- | :--- |
| Bootstrap | `main.go`, `bootstrap/` | Memuat konfigurasi, membuka koneksi DB, menjalankan HTTP server dan Reaper. |
| Workflow management | `internal/workflow/`, `internal/validator/` | Validasi DAG, registrasi definisi, versioning, dan diff versi. |
| Execution engine | `internal/execution/`, `internal/condition/` | Snapshot step, evaluasi `when`, interpolasi payload, transisi status, retry, dan Saga. |
| Worker protocol | `internal/worker/` | Autentikasi service, otorisasi queue, long polling, heartbeat, dan laporan hasil. |
| Reaper | `internal/reaper/` | Menutup attempt kedaluwarsa dan menjalankan kebijakan recovery. |
| Storage | MySQL 8.4, `migrations/` | Menyimpan definisi, execution, receipt idempotensi, dan riwayat attempt. |
| External worker | Project terpisah, misalnya `relay-worker-node` | Mengambil task melalui HTTP dan menjalankan HTTP/message/activity executor. |
| Dashboard | `web/src/`, `internal/web/` | Visualisasi workflow, execution, dan metrik; hasil build di-embed ke binary Go. |

### 3.2 Diagram Arsitektur Keseluruhan

```mermaid
flowchart LR
    Client[Client service] -->|Register / execute| API[Relay Go API]
    UI[React dashboard] -->|Read status| API
    API <-->|Transactions| DB[(MySQL 8.4)]
    Reaper[Go Reaper] -->|Recover expired attempts| DB
    Worker[relay-worker-node] -->|Claim / heartbeat / complete / fail| API
    Worker --> HTTP[HTTP services]
    Worker --> MQ[RabbitMQ]
    Worker --> Activity[Activity handlers]
```

Worker tidak membutuhkan akses langsung ke database Relay. RabbitMQ hanya dibutuhkan ketika worker mengimplementasikan executor `message`; broker tersebut bukan antrean internal engine.

### 3.3 Diagram Alur Eksekusi & Siklus Hidup Worker

```mermaid
sequenceDiagram
    participant Client
    participant API as Relay API
    participant DB as MySQL
    participant Worker as External worker
    participant Target as Target service
    Client->>API: POST /workflows/{name}/execute + Idempotency-Key
    API->>DB: Save execution, step snapshots, receipt
    API-->>Client: 201 Created, execution id
    Worker->>API: POST /worker/tasks/claim + Bearer token
    API->>DB: Claim matching PENDING task in transaction
    API-->>Worker: 200 task + lease_token + resolved payload
    par Execute
        Worker->>Target: Execute HTTP / publish message / run activity
        Target-->>Worker: Result
    and Maintain lease
        Worker->>API: POST /worker/tasks/{id}/heartbeat
        API->>DB: Extend lease up to attempt deadline
        API-->>Worker: 200 lease_expires_at
    end
    Worker->>API: POST /worker/tasks/{id}/complete
    API->>DB: Save output and reconcile DAG in transaction
    API-->>Worker: 200 accepted=true
```

Jika claim tidak menemukan task sampai budget long poll habis, response adalah `204 No Content` tanpa body. Worker kemudian melakukan claim berikutnya.

### 3.4 Diagram State Machine (Workflow & Step Lifecycle)

```mermaid
stateDiagram-v2
    [*] --> WAITING
    WAITING --> PENDING: Dependencies COMPLETED or SKIPPED
    PENDING --> RUNNING: Claim succeeds, condition true
    PENDING --> SKIPPED: Condition false
    PENDING --> FAILED: Condition evaluation error
    RUNNING --> COMPLETED: Normal task complete
    RUNNING --> COMPENSATED: Compensation task complete
    RUNNING --> PENDING: Retryable failure / lease expiry, attempts remain
    RUNNING --> FAILED: Normal task final failure
    RUNNING --> COMPENSATION_FAILED: Compensation final failure
    WAITING --> CANCELLED: Workflow failure / compensation unnecessary
    PENDING --> CANCELLED: Workflow failure / compensation unnecessary
```

Step tanpa dependency langsung dibuat `PENDING`. Workflow normal selesai sebagai `COMPLETED` ketika seluruh step normal `COMPLETED` atau `SKIPPED`. Kegagalan dapat memindahkan workflow ke `COMPENSATING`, lalu `COMPENSATED`, `COMPENSATION_FAILED`, atau `FAILED` sesuai hasil rekonsiliasi. Kompensasi yang sukses dicatat sebagai `COMPENSATED` pada step, sedangkan outcome attempt-nya `COMPLETED`.

### 3.5 Detail Mekanisme Kunci Engine

- **Atomic claim:** transaksi singkat dengan `FOR UPDATE SKIP LOCKED` mengunci workflow dan task kandidat. Lock database dilepas sebelum menunggu long poll atau menjalankan pekerjaan bisnis.
- **Snapshot dan routing:** konfigurasi step disalin saat execution dibuat. Claim difilter berdasarkan `task_queue`, `executor_types`, dan daftar `activities` yang didukung worker.
- **Lease dan deadline:** heartbeat memperpanjang lease sampai deadline attempt. Lease yang sudah habis tidak dapat dihidupkan kembali oleh worker lama.
- **Recovery:** Reaper menandai attempt sebagai `ABANDONED` jika lease habis sebelum deadline, atau `TIMED_OUT` jika deadline sudah tercapai. Engine kemudian menentukan retry atau kegagalan final berdasarkan policy.
- **Dependency dan kondisi:** dependency `COMPLETED`/`SKIPPED` membuka step downstream. Evaluasi `when` dan interpolasi config dilakukan backend sebelum task diberikan kepada worker.
- **Saga:** workflow kompensasi diprioritaskan saat claim. Urutan kompensasi mengikuti dependensi balik; worker menjalankan payload kompensasi yang diberikan backend.
- **Idempotensi:** pembuatan execution memakai `Idempotency-Key`. Report complete/fail memakai lease token dan hash payload pada riwayat attempt. Efek bisnis pada target service tetap memerlukan idempotensi tersendiri.

Dashboard disajikan bersama API di `http://localhost:8000/`. Untuk development frontend:

```bash
cd web
npm run dev
# Build ke internal/web/dist sebelum membangun ulang binary Go:
npm run build
```

Saat memakai Vite dev server, sesuaikan proxy di `web/vite.config.ts` dengan port backend. Konfigurasi saat ini menunjuk `8080`, sedangkan default Go adalah `8000`; tambahkan proxy `/system/metrics` untuk metrik dashboard saat development.

---

## 4. relay-worker-node — Panduan Implementasi

Bagian ini adalah spesifikasi awal untuk membangun **project terpisah `relay-worker-node`** berdasarkan Worker API yang sudah tersedia di backend ini. Struktur folder, konfigurasi worker, dan API handler di bawah merupakan rancangan; runtime Node.js belum diimplementasikan dalam repo Relay ini.

### 4.1 Scope dan Struktur Project

`relay-worker-node` bertugas mengambil task, menjalankan executor, menjaga lease, dan melaporkan hasil. Backend Go tetap mengatur DAG, `when`, interpolasi, retry policy, versioning, dan kompensasi.

Struktur yang dapat dipakai:

```text
relay-worker-node/
  src/
    index.ts                 # Bootstrap dan signal shutdown
    config.ts                # Parsing dan validasi environment
    client/relay-client.ts   # HTTP Worker API dan error terstruktur
    runtime/poller.ts        # Claim sesuai kapasitas concurrency
    runtime/task-runner.ts   # Eksekusi, heartbeat, deadline, dan report
    executors/http.ts        # HTTP request executor
    executors/message.ts     # RabbitMQ publisher
    executors/activity.ts    # Dispatch activity registry
    activities/index.ts     # Handler bisnis yang didaftarkan aplikasi
    types.ts                 # Task, handler context, dan report
  test/
  .env.example
  package.json
  tsconfig.json
  README.md
```

Implementasi awal dapat mendukung `activity` terlebih dahulu. Worker hanya mengiklankan executor dan activity yang sudah tersedia. Untuk concurrency `N`, jalankan maksimal `N` task aktif; ambil task baru ketika ada slot, tanpa menumpuk task yang lease-nya sudah berjalan di antrean lokal.

### 4.2 Konfigurasi Backend dan Worker

**Backend Relay Go:** terapkan seluruh migrasi `.up.sql` berurutan sampai `000008_add_external_worker_support` pada database yang dipakai. Startup aplikasi tidak menerapkan migrasi secara otomatis. Untuk database existing, terapkan hanya migrasi yang belum dijalankan.

Tambahkan konfigurasi berikut pada environment backend, di samping konfigurasi database:

```dotenv
PORT=8000
RELAY_WORKER_API_ENABLED=true
RELAY_WORKER_CREDENTIALS='[{"id":"orders-service","token":"replace-with-worker-secret","queues":["orders"]}]'
WORKER_LONG_POLL_TIMEOUT=25s
DEFAULT_LEASE_DURATION=30s
DEFAULT_HEARTBEAT_INTERVAL=10s
DEFAULT_TIMEOUT=15s
REAPER_INTERVAL=5s
```

Contoh menggunakan format `.env` yang dibaca `godotenv`. Saat menyuntikkan environment melalui deployment, nilai credentials adalah array JSON tersebut tanpa pembungkus petik shell. Worker API default **nonaktif**; mengisi credentials saja tidak mengaktifkannya. Saat diaktifkan, credentials wajib nonkosong dan setiap entry wajib memiliki `id` serta `token`. `queues` menentukan izin; `"*"` memberikan akses semua queue.

`id` adalah identitas service hasil autentikasi Bearer token. `worker_id` adalah identitas instance/proses yang dikirim worker dan harus tetap sama selama claim, heartbeat, dan report task itu.

**Konvensi environment yang diusulkan untuk `relay-worker-node`:**

| Variable | Contoh/default rancangan | Fungsi |
| :--- | :--- | :--- |
| `RELAY_BASE_URL` | `http://localhost:8000` | Alamat API Go. |
| `RELAY_WORKER_TOKEN` | Wajib diisi | Token yang cocok dengan credentials backend. |
| `RELAY_WORKER_ID` | `orders-node-01` | ID unik per instance, misalnya hostname dan UUID proses. |
| `RELAY_TASK_QUEUE` | `orders` | Satu queue per poller. |
| `RELAY_EXECUTOR_TYPES` | `activity` | Daftar tipe dipisahkan koma. |
| `RELAY_ACTIVITIES` | `inventory.reserve,inventory.release` | Daftar handler yang terdaftar. |
| `RELAY_CONCURRENCY` | `1` | Maksimal task aktif. |
| `RELAY_WAIT_TIMEOUT_MS` | `25000` | Tidak boleh melebihi batas long poll backend. |
| `RELAY_REQUEST_TIMEOUT_MS` | `35000` | Timeout request claim, lebih panjang dari long poll. |
| `RELAY_SHUTDOWN_TIMEOUT_MS` | `30000` | Budget drain task saat shutdown. |

Nama environment worker di atas bukan environment yang otomatis dibaca backend. Validasi konfigurasi saat startup, termasuk queue, token, handler yang diiklankan, concurrency positif, dan hubungan timeout. Request heartbeat/report perlu timeout tersendiri yang muat dalam sisa lease/deadline.

### 4.3 Kontrak HTTP Worker API

Semua endpoint berikut menggunakan `POST`, header `Content-Type: application/json`, dan `Authorization: Bearer <RELAY_WORKER_TOKEN>`. Worker API tidak memakai header `Idempotency-Key`; kepemilikan attempt dibuktikan dengan `worker_id` dan `lease_token`.

#### Claim task

```http
POST /worker/tasks/claim
Authorization: Bearer replace-with-worker-secret
Content-Type: application/json

{
  "worker_id": "orders-node-01",
  "queue": "orders",
  "executor_types": ["activity"],
  "activities": ["inventory.reserve", "inventory.release"],
  "wait_timeout_ms": 25000
}
```

- `worker_id` dan `queue` wajib nonkosong. Queue kosong atau tidak valid menghasilkan `400 BAD_REQUEST` sebelum pemeriksaan izin queue.
- `executor_types` kosong memakai filter backend `http`, `message`, `activity`. Kirim daftar eksplisit sesuai kemampuan worker.
- **Activity hanya bisa diambil jika namanya ada di `activities`.** Daftar kosong tidak berarti semua activity.
- `wait_timeout_ms` tidak diisi memakai default 25.000 ms, dibatasi konfigurasi backend. Nilai `0` berarti satu percobaan langsung; nilai negatif atau melebihi batas menghasilkan `400`.
- `200 OK` berisi task; `204 No Content` berarti tidak ada task sampai budget tunggu habis. Jangan memanggil `response.json()` pada response `204`.

Contoh response `200 OK` (UUID, waktu, dan token hanya ilustrasi):

```json
{
  "task_id": "2f6928f7-31cd-47bb-a5b1-980c1ff9914d",
  "workflow_execution_id": "afd70011-56d5-45f6-a590-bf514bf2091d",
  "step_id": "reserve-stock",
  "type": "activity",
  "task_queue": "orders",
  "activity_name": "inventory.reserve",
  "attempt": 1,
  "is_compensation": false,
  "timeout_seconds": 60,
  "deadline": "2026-11-04T10:01:00Z",
  "heartbeat_interval_ms": 10000,
  "lease_token": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "lease_expires_at": "2026-11-04T10:00:30Z",
  "lease_duration_ms": 30000,
  "payload": {
    "queue": "orders",
    "activity": "inventory.reserve",
    "input": { "order_id": "ORD-1001" }
  }
}
```

Gunakan `task_id` sebagai `{id}` pada endpoint task. `payload` adalah **seluruh config yang sudah diinterpolasi backend**, bukan hanya `input`. Worker tidak perlu mengambil definisi workflow atau mengevaluasi ulang `when`.

#### Heartbeat

```http
POST /worker/tasks/2f6928f7-31cd-47bb-a5b1-980c1ff9914d/heartbeat
Authorization: Bearer replace-with-worker-secret
Content-Type: application/json

{
  "worker_id": "orders-node-01",
  "lease_token": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "lease_duration_ms": 30000
}
```

Response `200 OK`:

```json
{
  "lease_expires_at": "2026-11-04T10:00:40Z",
  "lease_duration_ms": 30000
}
```

`lease_duration_ms` pada request opsional. Durasi yang diminta tidak dapat memperpanjang lease melebihi default backend, dan lease selalu dibatasi deadline attempt. Gunakan **`lease_expires_at` sebagai batas aktual**, karena durasi nominal pada response dapat lebih panjang daripada waktu yang tersisa sebelum deadline.

Jadwalkan heartbeat tanpa request yang overlap, sebelum lease kedaluwarsa, dengan mengacu pada `heartbeat_interval_ms` dan sisa lease. Sisakan margin untuk latensi dan report akhir. Heartbeat tidak memperpanjang deadline. Timeout `0` pada definisi step tetap memakai default backend, bukan durasi tak terbatas.

#### Complete

```http
POST /worker/tasks/2f6928f7-31cd-47bb-a5b1-980c1ff9914d/complete
Authorization: Bearer replace-with-worker-secret
Content-Type: application/json

{
  "worker_id": "orders-node-01",
  "lease_token": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "output": { "reservation_id": "RES-1001" }
}
```

Response `200 OK`: `{"accepted": true}`. Output kosong atau `null` dinormalisasi backend menjadi `{}`. Output normal tersedia hanya melalui `steps.<step_id>.output`, misalnya `{{steps.reserve-stock.output.reservation_id}}`.

#### Fail

```http
POST /worker/tasks/2f6928f7-31cd-47bb-a5b1-980c1ff9914d/fail
Authorization: Bearer replace-with-worker-secret
Content-Type: application/json

{
  "worker_id": "orders-node-01",
  "lease_token": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "error": {
    "message": "Inventory service unavailable",
    "retryable": true
  }
}
```

Response `200 OK`: `{"accepted": true}`. `retryable` default `true` jika tidak dikirim; kirim secara eksplisit. Backend memutuskan apakah masih ada attempt dan menghitung backoff. Worker tidak menjadwalkan ulang workflow sendiri. Untuk error permanen seperti input bisnis invalid atau handler tidak dikenal, gunakan `retryable: false`.

#### Error dan replay report

Format error: `{"error": "...", "code": "LOCK_LOST"}`.

| HTTP / code | Arti dan tindakan worker |
| :--- | :--- |
| `400 BAD_REQUEST` | Payload atau wait timeout invalid; perbaiki request. |
| `401 UNAUTHORIZED` | Bearer token tidak valid; hentikan polling sampai konfigurasi diperbaiki. |
| `401 INVALID_LEASE_TOKEN` | Token attempt salah/tidak dikenal; jangan lanjutkan task dengan token tersebut. |
| `403 WORKER_API_DISABLED` | Aktifkan API pada environment backend. |
| `403 FORBIDDEN_QUEUE` / `FORBIDDEN_TASK` | Service tidak memiliki akses queue atau attempt itu. |
| `404 NOT_FOUND` | Task tidak ditemukan. |
| `409 LOCK_LOST` | Worker/attempt tidak lagi memiliki lease yang dapat dipakai; batalkan pekerjaan yang masih berjalan. |
| `409 REPORT_CONFLICT` | Outcome sejenis sudah diterima dengan payload berbeda; jangan ubah report. |
| `413 PAYLOAD_TOO_LARGE` | Body melebihi batas backend, default 5 MiB. |
| `500 INTERNAL_ERROR` / gangguan jaringan | Retry request dengan backoff dan jitter sesuai budget operasi. |

Complete/fail yang **sudah diterima** dapat dikirim ulang memakai service, worker ID, lease token, jenis report, dan payload yang sama, termasuk setelah lease berakhir. Backend mengembalikan `accepted: true` tanpa mengulang transisi. Perbedaan urutan key JSON tidak menjadi konflik. Report pertama yang belum diterima membutuhkan lease dan deadline aktif. Beralih dari complete ke fail atau sebaliknya pada attempt yang sudah selesai ditolak sebagai `LOCK_LOST`.

### 4.4 Executor dan Activity Registry

| Tipe | Input dari task | Perilaku worker yang perlu dibuat |
| :--- | :--- | :--- |
| `http` | `payload.method`, `url`, `headers`, `body` | Panggil target dengan abort/deadline. Tentukan klasifikasi status HTTP dan bentuk output secara eksplisit. |
| `message` | `payload.exchange`, `routing_key`, `content_type`, `headers`, `payload` | Publish ke RabbitMQ. Gunakan publisher confirms sebagai dasar keberhasilan publish; ini tidak berarti consumer telah selesai. |
| `activity` | `activity_name`, `payload.input` | Cari handler pada registry dan jalankan handler tersebut. |

Bentuk output HTTP/message adalah kontrak project worker yang harus ditetapkan agar referensi output workflow konsisten. Misalnya, HTTP dapat mengembalikan body JSON secara langsung, sedangkan message mengembalikan metadata publish. Backend tidak memanggil target atau mendefinisikan output executor untuk worker.

Contoh kontrak handler TypeScript yang diusulkan:

```typescript
type Json = null | boolean | number | string | Json[] | { [key: string]: Json };

type ActivityContext = {
  taskId: string;
  workflowExecutionId: string;
  stepId: string;
  attempt: number;
  isCompensation: boolean;
  signal: AbortSignal;
  idempotencyKey: string;
};

type ActivityHandler = (input: Json, ctx: ActivityContext) => Promise<Json>;
type ActivityRegistry = Record<string, ActivityHandler>;
```

Activity registry mengiklankan seluruh handler normal **dan kompensasi** yang mampu dijalankan. Kompensasi diterima sebagai task biasa dengan `is_compensation: true`, payload kompensasi, dan lease sendiri. Pada definisi kompensasi, `{{this.output.reservation_id}}` merujuk output step normal yang sedang dikompensasi; backend sudah me-resolve nilainya sebelum claim.

Efek bisnis dapat terulang setelah crash atau lease expiry, tergantung retry policy. Gunakan `task_id` sebagai dasar idempotency key yang stabil lintas attempt; jangan memasukkan `attempt` untuk operasi bisnis yang sama. Task kompensasi mempunyai ID terpisah. Jika target menggunakan key bisnis seperti `order_id`, koordinasikan scope key tersebut dengan kontrak target.

ID numerik di atas `Number.MAX_SAFE_INTEGER` perlu dikirim sebagai string dalam kontrak aplikasi agar tidak dibulatkan saat JSON diparse Node.js. Output handler harus JSON-serializable; `BigInt` tidak dapat langsung dikirim dengan `JSON.stringify`.

### 4.5 Lifecycle Worker dan Penanganan Kegagalan

```text
Saat ada slot concurrency:
  claim task sesuai queue dan capabilities
  jika 204: lanjutkan long poll berikutnya
  jika task tersedia:
    mulai pengawasan lease/deadline dan heartbeat tanpa overlap
    dispatch executor dengan AbortSignal
    jika pekerjaan sukses: bekukan report COMPLETE beserta output
    jika pekerjaan gagal: bekukan report FAIL beserta error/retryable
    kirim report; retry hanya pengiriman report yang sama jika respons tidak pasti
    hentikan pengawasan setelah report diterima atau attempt tak dapat dilanjutkan
    bebaskan slot
```

Aturan runtime:

1. **Pisahkan error eksekusi dan error pengiriman report.** Jika pekerjaan sukses tetapi request complete timeout, ulangi complete yang sama. Jangan menjalankan ulang handler atau mengirim fail hanya karena request report bermasalah.
2. **Bekukan report sekali.** Simpan body yang sudah diserialisasi agar retry tidak menambahkan timestamp/error baru. Jangan berpindah jenis report untuk attempt yang sama.
3. **Jaga lease sampai report terselesaikan.** Heartbeat dan retry transport memakai budget sisa lease/deadline. Jika heartbeat mengembalikan `LOCK_LOST`, hentikan efek bisnis baru dan abort operasi yang mendukung pembatalan. Report yang responsnya tidak pasti masih dapat di-replay identik untuk mengecek apakah sebelumnya sudah diterima.
4. **Gunakan batas waktu lokal.** Hentikan handler sebelum deadline dengan margin untuk report. Abort bersifat kooperatif; handler harus meneruskan signal ke operasi I/O. Pekerjaan CPU yang memblokir event loop perlu diisolasi agar heartbeat tidak terhambat.
5. **Retry transport berbeda dari retry bisnis.** Gangguan claim/heartbeat/report dapat dicoba ulang dengan backoff pendek dan jitter. Retry bisnis tetap ditentukan Relay melalui report fail atau Reaper.
6. **Jangan log Bearer token atau lease token.** Gunakan task ID, execution ID, attempt, worker ID, dan durasi sebagai metadata log.

Saat `SIGINT`/`SIGTERM`, hentikan claim baru dan batalkan long poll yang menunggu. Jika respons claim sedang berpacu dengan shutdown, jangan mulai efek bisnis baru setelah mode drain aktif. Drain task yang sudah berjalan sambil mempertahankan heartbeat dan report sampai budget shutdown habis. Setelah itu abort pekerjaan dan hentikan heartbeat; Reaper menangani lease kedaluwarsa sesuai retry policy. Tidak ada endpoint release/nack pada protokol saat ini. Claim yang responsnya hilang juga dapat meninggalkan lease sementara yang dipulihkan Reaper.

### 4.6 Contoh HTTP Client Node.js

Contoh ESM berikut memakai `fetch` bawaan Node.js. Simpan sebagai `relay-client.mjs` di project worker. Ini adalah lapisan transport; poller, executor, heartbeat loop, retry, dan shutdown dibuat terpisah mengikuti lifecycle di atas.

```javascript
export class RelayApiError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export class RelayClient {
  constructor({ baseUrl, token, workerId }) {
    if (!baseUrl || !token || !workerId) {
      throw new Error('baseUrl, token, dan workerId wajib diisi');
    }
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.token = token;
    this.workerId = workerId;
  }

  // string body juga diterima untuk replay report yang sudah dibekukan.
  async post(path, body, { signal, timeoutMs = 5000 } = {}) {
    const response = await fetch(`${this.baseUrl}${path}`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${this.token}`,
        'Content-Type': 'application/json',
      },
      body: typeof body === 'string' ? body : JSON.stringify(body),
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(timeoutMs)])
        : AbortSignal.timeout(timeoutMs),
    });

    if (response.status === 204) return null;
    const raw = await response.text();
    let data;
    try {
      data = JSON.parse(raw);
    } catch {
      // Tetap pertahankan status saat proxy mengembalikan body non-JSON.
      throw new RelayApiError(response.status, 'INVALID_RESPONSE', 'Expected JSON response');
    }
    if (!response.ok) {
      throw new RelayApiError(response.status, data.code, data.error);
    }
    return data;
  }

  claim({ queue, executorTypes, activities, waitTimeoutMs = 25000 }, options = {}) {
    return this.post('/worker/tasks/claim', {
      worker_id: this.workerId,
      queue,
      executor_types: executorTypes,
      activities,
      wait_timeout_ms: waitTimeoutMs,
    }, { timeoutMs: waitTimeoutMs + 10000, ...options });
  }

  heartbeat(task, options) {
    return this.post(`/worker/tasks/${encodeURIComponent(task.task_id)}/heartbeat`, {
      worker_id: this.workerId,
      lease_token: task.lease_token,
    }, options);
  }

  // Buat sekali setelah eksekusi, lalu simpan dan kirim ulang objek report ini.
  prepareReport(task, type, result) {
    if (type !== 'complete' && type !== 'fail') {
      throw new Error('Report type harus complete atau fail');
    }
    return Object.freeze({
      path: `/worker/tasks/${encodeURIComponent(task.task_id)}/${type}`,
      body: JSON.stringify({
        worker_id: this.workerId,
        lease_token: task.lease_token,
        ...(type === 'complete' ? { output: result } : { error: result }),
      }),
    });
  }

  report(prepared, options) {
    return this.post(prepared.path, prepared.body, options);
  }
}
```

Contoh pemakaian oleh runtime setelah handler berhasil:

```javascript
const report = client.prepareReport(task, 'complete', { reservation_id: 'RES-1001' });
await client.report(report, { timeoutMs: 3000 });
// Jika respons hilang, retry client.report(report) dengan body yang sama.
```

Untuk kegagalan handler, result report fail berbentuk `{ message: '...', retryable: true }`. Sesuaikan timeout tiap request dengan sisa lease/deadline; contoh client tidak menghitung budget itu secara otomatis.

### 4.7 Smoke Test Integrasi

Jalankan backend dengan konfigurasi di Bagian 4.2, lalu daftarkan handler `inventory.reserve` dan `inventory.release` pada worker queue `orders`. Contoh berikut memakai Bash/cURL; pada PowerShell gunakan `curl.exe` dan sesuaikan quoting shell.

**1. Daftarkan workflow activity dengan kompensasi:**

```bash
curl -X POST http://localhost:8000/workflows \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "orders.reserve",
    "version": 1,
    "steps": [{
      "id": "reserve-stock",
      "type": "activity",
      "timeout": 60,
      "config": {
        "queue": "orders",
        "activity": "inventory.reserve",
        "input": { "order_id": "{{context.order_id}}" }
      },
      "retry": { "max_attempts": 3, "backoff": 2, "multiplier": 2, "max_backoff": 30 },
      "compensation": {
        "type": "activity",
        "timeout": 60,
        "config": {
          "queue": "orders",
          "activity": "inventory.release",
          "input": { "reservation_id": "{{this.output.reservation_id}}" }
        }
      }
    }]
  }'
```

**2. Jalankan versi eksplisit:**

```bash
curl -X POST http://localhost:8000/workflows/orders.reserve/execute \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: orders.reserve:ORD-1001' \
  -d '{"workflow_version":1,"context":{"order_id":"ORD-1001"}}'
```

Gunakan nama baru atau versi yang dikembalikan saat registrasi jika workflow tersebut sudah ada dengan definisi lain. Gunakan idempotency key baru untuk setiap execution uji yang baru.

**3. Jalankan worker yang dibuat**, dengan filter claim `executor_types: ["activity"]`, `activities: ["inventory.reserve", "inventory.release"]`, dan queue `orders`. Untuk pemeriksaan manual tanpa runtime Node, gunakan kontrak claim/complete di Bagian 4.3, salin task ID dan lease token dari respons aktual, lalu laporkan hasil sebelum lease habis.

**4. Periksa execution:**

```bash
curl http://localhost:8000/executions/<execution-id-dari-response>
```

Handler reserve untuk smoke test dapat mengembalikan `{"reservation_id":"RES-1001"}`. Hasil yang diharapkan: workflow `COMPLETED`, step normal `COMPLETED`, dan satu entry `attempts` dengan outcome `COMPLETED`. Kompensasi menjadi `CANCELLED` karena tidak diperlukan.

Untuk menguji Saga, tambahkan step downstream yang bergantung pada `reserve-stock` lalu buat handler downstream gagal dengan `retryable: false`. Daftarkan juga activity downstream di worker. Hasil yang diharapkan: `inventory.release` menerima reservation ID dari output awal dan workflow berakhir `COMPENSATED` jika kompensasi berhasil.

Detail execution mengekspos riwayat attempt seperti `service_id`, `worker_id`, waktu, deadline, outcome, dan error. Lease token serta hash token tidak diekspos pada endpoint detail execution.

### 4.8 Kriteria Penerimaan

- Claim idle menangani `204` tanpa parse JSON; timeout HTTP lebih panjang dari long poll.
- Queue yang tidak diizinkan ditolak; activity yang tidak diiklankan tidak diambil.
- Dua instance tidak mendapatkan lease aktif yang sama untuk satu attempt.
- Task yang berjalan lama mengirim heartbeat tanpa overlap, dan berhenti sebelum deadline.
- Concurrency dibatasi sebelum claim, sehingga tidak ada task leased yang menunggu slot lokal.
- Complete/fail identik dapat di-replay; payload berbeda ditolak tanpa mengubah hasil awal.
- Timeout complete tidak menjalankan ulang handler dan tidak mengirim fail pengganti.
- Handler yang kehilangan lease menghentikan pekerjaan kooperatif; Reaper menentukan recovery.
- Retry bisnis mengikuti policy backend, dan idempotensi target mencegah efek bisnis ganda.
- Task kompensasi masuk ke handler yang benar dan melaporkan complete/fail dengan lease sendiri.
- Shutdown menghentikan claim, melakukan drain terbatas, dan membersihkan timer/heartbeat.
- Pengujian transport memakai fake server; pengujian lifecycle dan konkurensi memakai backend Relay serta database tes terisolasi.

Referensi implementasi backend: [`internal/worker/model.go`](internal/worker/model.go), [`internal/worker/handler.go`](internal/worker/handler.go), [`internal/worker/logic.go`](internal/worker/logic.go), [`internal/execution/repository_lease.go`](internal/execution/repository_lease.go), dan [`internal/execution/repository_lifecycle.go`](internal/execution/repository_lifecycle.go).

### Breaking contract cleanup

Queue harus eksplisit pada definisi normal, kompensasi, dan request claim. Bentuk execute raw-context, alias output root step, namespace kompensasi `original`, serta alias claim `step_execution_id` sudah dihapus. Gunakan `steps.<id>.output`, `this.output`, dan `task_id`.

Migrasi `000009` menghapus default `step_executions.task_queue` tanpa mengubah data lama. Selesaikan execution yang memakai kontrak lama sebelum cutover engine, SDK, dan definisi workflow baru. Tidak ada konversi otomatis workflow atau data.
