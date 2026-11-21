---
title: Qui mange mon trafic VyOS ? Ce petit outil le montre d'un coup d'œil 😭
date: 2026-11-26
summary: Le réseau du bureau rampe aux heures de pointe et personne ne sait qui mange la bande passante ? Un seul binaire règle ça : bande passante, top talkers et enregistrements de flux, pour tout ce qui exporte du sFlow, dans le navigateur ou en SSH. En route en 30 secondes.
---

D'abord, la douleur 👇

Le VyOS à la sortie de notre bureau tourne depuis longtemps.
Chaque jour ouvré, à l'heure de pointe, le réseau rampe.
Qui mange la bande passante ? Aucune idée.
Quelle machine, qui parle à quel serveur ? Aucune idée non plus.
Que des devinettes. La réponse, c'était toujours « essaie de redémarrer » 🙂

VyOS peut exporter des échantillons sFlow.
La plupart des commutateurs, des routeurs d'autres marques et des serveurs aussi.

Mais après ? Il n'y avait rien pour les regarder ❌
Les outils habituels demandent un serveur séparé, ou une base de données et toute une pile de tableaux de bord.
Beaucoup trop, juste pour voir mon propre trafic.

Alors j'ai écrit un petit outil moi-même. Il s'appelle vyos-sflow ✨
Un seul binaire.
Il tourne sur le routeur lui-même,
ou mettez-le sur n'importe quelle machine Linux et tous les équipements y envoient leur sFlow.

## 📦 Installation en trois étapes

D'abord, activez sFlow dans VyOS (sautez cette étape s'il envoie déjà vers 127.0.0.1:6343) :

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

Pas de VyOS ? Même principe : pointez l'export sFlow de cet équipement vers cette machine, port 6343, et continuez ci-dessous.

Ensuite, téléchargez-le et lancez-le :

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Ouvrez `http://ROUTER_IP:8066` dans un navigateur, connectez-vous avec admin / tryvyossflow66, et changez le mot de passe.
C'est tout. Les graphiques commencent à se remplir.

![Page d'aperçu](assets/img/screenshots/vyos/web-overview.png)

## 👀 Ce que vous obtenez

✅ La bande passante actuelle, et la courbe des dernières 48 heures

✅ Top talkers : quel client et quel serveur utilisent le plus de trafic, classés

✅ Le trafic des interfaces depuis les compteurs de l'équipement lui-même, à côté de l'estimation échantillonnée, pour voir si les chiffres concordent

✅ Les enregistrements de flux derrière les graphiques : cliquez un pic et découvrez qui c'était

✅ Si un graphique est vide, la page Sources vous dit pourquoi. Fini les devinettes

![Top talkers](assets/img/screenshots/vyos/web-top.png)

## 🖥️ Pas de navigateur ? Utilisez le terminal

Connectez-vous au routeur en SSH et lancez une seule commande :

```
./vyos-sflow tui
```

La même image, dans votre terminal. Appuyez sur q pour quitter.
Rien ne s'installe, parfait pour un coup d'œil rapide.

Petits extras : l'interface web parle 15 langues et propose cinq thèmes, du clair au sombre.

## ⚠️ Les limites, honnêtement

- sFlow uniquement. Pas de NetFlow, pas d'IPFIX
- L'historique couvre 48 heures. Il répond à « que se passe-t-il » et « que vient-il de se passer », pas aux rapports à long terme
- Linux amd64 uniquement. Cela couvre VyOS et les serveurs Linux ordinaires, mais il n'y a pas de version Windows ni macOS

Si ces trois points vous conviennent, il fait bien le travail.

## 🔗 Récupérez-le

Gratuit et open source, Apache-2.0.
Cherchez vyos-sflow sur GitHub, ou utilisez le lien de téléchargement de ce site.
S'il ne vous plaît pas, supprimez le dossier et il disparaît — le routeur lui-même n'est pas touché.

Si VyOS sert de sortie à votre bureau ou de passerelle à un cybercafé, essayez-le.
Ce sentiment de « où est passé mon trafic » disparaît.

Et vous, vous utilisez quoi pour surveiller votre trafic de sortie ? Dites-le-moi en dessous 👇

#VyOS #sFlow #opensource #réseau #sysadmin
