---
title: Siapa yang makan traffic VyOS saya? Alat kecil ini langsung kelihatan semua 😭
date: 2026-11-30
summary: Jaringan kantor lemot tiap jam sibuk dan nggak ada yang tahu siapa yang makan bandwidth? Satu binary beres: bandwidth, top talkers, dan flow records, buat semua perangkat yang bisa ekspor sFlow, di browser atau lewat SSH. Jalan dalam 30 detik.
---

Sakitnya dulu ya 👇

VyOS di pintu keluar kantor kami sudah lama menyala.
Tiap jam sibuk hari kerja, jaringan lemot.
Siapa yang makan bandwidth? Nggak tahu.
Mesin mana, ngobrol sama server mana? Nggak tahu juga.
Cuma nebak-nebak. Jawabannya selalu "coba restart" 🙂

VyOS bisa mengekspor sampel sFlow.
Kebanyakan switch, router merek lain, dan server juga bisa.

Terus setelah itu? Nggak ada alat ringan buat melihatnya ❌
Alat yang biasa dipakai minta server terpisah, atau database plus tumpukan dashboard.
Terlalu berat, cuma buat lihat traffic sendiri.

Makanya saya buat sendiri alat kecil ini. Namanya vyos-sflow ✨
Satu binary.
Jalan langsung di routernya,
atau taruh di mesin Linux mana pun, biar semua perangkat kirim sFlow ke situ.

## 📦 Tiga langkah untuk memasang

Pertama, nyalakan sFlow di VyOS (lewati kalau sudah kirim ke 127.0.0.1:6343):

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

Perangkatnya bukan VyOS? Idenya sama: arahkan ekspor sFlow perangkat itu ke mesin ini, port 6343, lalu lanjut ke bawah.

Lalu download dan jalankan:

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Buka `http://IP_ROUTER:8066` di browser, login dengan admin / tryvyossflow66, lalu ganti password-nya.
Itu saja. Grafiknya mulai terisi.

![Halaman overview](assets/img/screenshots/vyos/web-overview.png)

## 👀 Yang kamu dapatkan

✅ Bandwidth sekarang, plus kurva 48 jam terakhir

✅ Top talkers: client dan server mana yang paling banyak pakai traffic, terurut

✅ Traffic interface dari counter perangkat itu sendiri, di samping estimasi sampling — kelihatan cocok atau tidak angkanya

✅ Flow records di balik grafiknya: klik satu lonjakan, ketahuan siapa pelakunya

✅ Kalau grafik kosong, halaman Sources kasih tahu kenapa. Nggak perlu nebak

![Top talkers](assets/img/screenshots/vyos/web-top.png)

## 🖥️ Nggak pakai browser? Pakai terminal

SSH ke router dan jalankan satu perintah:

```
./vyos-sflow tui
```

Tampilan yang sama, di terminal kamu. Tekan q buat keluar.
Nggak ada yang diinstal, enak buat cek cepat.

Bonus kecil: tampilan webnya punya 15 bahasa dan lima tema, dari terang sampai gelap.

## ⚠️ Batasannya, jujur saja

- Cuma sFlow. NetFlow dan IPFIX nggak bisa
- Riwayatnya 48 jam. Buat menjawab "sekarang lagi apa" dan "tadi barusan apa", bukan laporan jangka panjang
- Cuma Linux amd64. Cukup buat VyOS dan server Linux biasa, tapi nggak ada versi Windows atau macOS

Kalau tiga hal itu oke buat kamu, alat ini kerja dengan baik.

## 🔗 Ambil di sini

Gratis dan open source, Apache-2.0.
Cari vyos-sflow di GitHub, atau pakai link download di situs ini.
Nggak suka? Hapus foldernya, selesai — routernya sendiri nggak disentuh.

Kalau VyOS jadi pintu keluar kantor atau gateway warnet, cobain deh.
Perasaan "traffic saya lari ke mana?" itu hilang.

Kalau kamu, pakai apa buat mantau traffic di pintu keluar? Cerita di bawah ya 👇

#VyOS #sFlow #opensource #jaringan #sysadmin
