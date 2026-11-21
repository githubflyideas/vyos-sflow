---
title: Wer frisst meinen VyOS-Traffic? Dieses kleine Tool zeigt es auf einen Blick 😭
date: 2026-12-02
summary: Jeden Werktag zur Stoßzeit kriecht das Büro-Netz, und keiner weiß, wer die Bandbreite frisst? Eine einzige Datei schafft Abhilfe: Bandbreite, Top Talker und Flow-Datensätze für alles, was sFlow exportiert — im Browser oder per SSH. In 30 Sekunden einsatzbereit.
---

Erst mal der Schmerz 👇

Die VyOS-Kiste an unserem Büro-Ausgang läuft schon lange.
Jeden Werktag zur Stoßzeit kriecht das Netz.
Wer frisst die Bandbreite? Keine Ahnung.
Welche Maschine, mit welchem Server? Auch keine Ahnung.
Nur Raten. Die Antwort war immer: „Neustart hilft bestimmt" 🙂

VyOS kann sFlow-Samples exportieren.
Die meisten Switches, anderen Router und Server können das auch.

Aber was dann? Es gab nichts, um sie anzuschauen ❌
Die üblichen Tools wollen einen eigenen Server oder eine Datenbank plus ganzen Dashboard-Stack.
Viel zu viel, nur um meinen eigenen Traffic zu sehen.

Also habe ich selbst ein kleines Tool geschrieben. Es heißt vyos-sflow ✨
Eine einzige Datei.
Auf dem Router selbst laufen lassen,
oder auf irgendeine Linux-Maschine legen, zu der alle Geräte ihr sFlow schicken.

## 📦 Installation in drei Schritten

Zuerst sFlow in VyOS einschalten (überspringen, falls es schon an 127.0.0.1:6343 sendet):

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

Kein VyOS-Gerät? Gleiches Spiel: den sFlow-Export des Geräts auf diese Maschine, Port 6343, zeigen lassen und unten weitermachen.

Dann herunterladen und starten:

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Im Browser `http://ROUTER_IP:8066` öffnen, mit admin / tryvyossflow66 anmelden und das Passwort ändern.
Das ist alles. Die Diagramme füllen sich.

![Overview page](assets/img/screenshots/vyos/web-overview.png)

## 👀 Das bekommst du

✅ Aktuelle Bandbreite und die Kurve der letzten 48 Stunden

✅ Top Talker: welcher Client und welcher Server den meisten Traffic nutzen, sortiert

✅ Interface-Traffic aus den eigenen Zählern des Geräts, direkt neben der Schätzung aus den Samples. So siehst du, ob die Zahlen zusammenpassen

✅ Die Flow-Datensätze hinter den Diagrammen — auf eine Spitze klicken und herausfinden, wer es war

✅ Wenn ein Diagramm leer ist, sagt dir die Sources-Seite warum. Kein Raten mehr

![Top talkers](assets/img/screenshots/vyos/web-top.png)

## 🖥️ Kein Browser? Nimm das Terminal

Per SSH auf den Router und einen Befehl ausführen:

```
./vyos-sflow tui
```

Das gleiche Bild, in deinem Terminal. Mit q beenden.
Es wird nichts installiert — nett für einen schnellen Blick.

Kleine Extras: Die Web-UI spricht 15 Sprachen und hat fünf Themes, von hell bis dunkel.

## ⚠️ Die ehrlichen Grenzen

- Nur sFlow. Kein NetFlow, kein IPFIX
- Verlauf sind 48 Stunden. Es beantwortet „was passiert gerade" und „was ist eben passiert", keine langfristigen Berichte
- Nur Linux amd64. Das deckt VyOS und normale Linux-Server ab, aber es gibt keinen Windows- oder macOS-Build

Wenn diese drei Punkte für dich okay sind, macht es seinen Job gut.

## 🔗 Hol es dir

Kostenlos und Open Source, Apache-2.0.
Auf GitHub nach vyos-sflow suchen oder den Download-Link auf dieser Seite nutzen.
Gefällt es dir nicht? Ordner löschen, und es ist weg — der Router selbst bleibt unverändert.

Wenn VyOS bei dir als Büro-Ausgang oder Internetcafé-Gateway läuft, probier es aus.
Dieses „wo ist mein Traffic hin"-Gefühl ist weg.

Womit schaust du dir deinen Ausgangs-Traffic an? Erzähl es unten 👇

#VyOS #sFlow #opensource #Netzwerk #Sysadmin
