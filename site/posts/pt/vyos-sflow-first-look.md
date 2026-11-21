---
title: Quem está comendo o tráfego do meu VyOS? Essa ferramentinha mostra tudo de cara 😭
date: 2026-11-28
summary: A rede do escritório arrasta no horário de pico e ninguém sabe quem está comendo a banda? Um binário só resolve: banda, ranking de quem mais usa e registros de fluxo, para qualquer equipamento que exporte sFlow, no navegador ou pelo SSH. Rodando em 30 segundos.
---

Primeiro a dor 👇

O VyOS na saída do nosso escritório está ligado há muito tempo.
Todo dia, no horário de pico, a rede arrasta.
Quem está comendo a banda? Não faço ideia.
Qual máquina, falando com qual servidor? Também não.
Só chute. A resposta era sempre "reinicia pra ver" 🙂

O VyOS exporta amostras sFlow.
A maioria dos switches, roteadores de outras marcas e servidores também.

Mas e aí? Não tinha nada leve para olhar esses dados ❌
As ferramentas de sempre pedem outro servidor, ou um banco de dados com um monte de painéis.
Coisa demais, só para ver o meu próprio tráfego.

Então eu mesmo escrevi uma ferramentinha. Ela se chama vyos-sflow ✨
Um único binário.
Roda no próprio roteador,
ou em qualquer máquina Linux, recebendo o sFlow de todos os equipamentos.

## 📦 Três passos para instalar

Primeiro, ligue o sFlow no VyOS (pule isto se ele já manda para 127.0.0.1:6343):

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

Não é VyOS? Mesma ideia: aponta a exportação sFlow daquele equipamento para esta máquina, porta 6343, e segue abaixo.

Depois baixe e rode:

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Abra `http://IP_DO_ROTEADOR:8066` no navegador, entre com admin / tryvyossflow66 e troque a senha.
Só isso. Os gráficos começam a encher.

![Página de visão geral](assets/img/screenshots/vyos/web-overview.png)

## 👀 O que você ganha

✅ Banda atual, e a curva das últimas 48 horas

✅ Ranking: qual cliente e qual servidor usam mais tráfego, em ordem

✅ Tráfego das interfaces pelos contadores do próprio equipamento, ao lado da estimativa por amostragem — dá pra ver se os números batem

✅ Os registros de fluxo por trás dos gráficos: clicou num pico, descobriu quem foi

✅ Se um gráfico estiver vazio, a página Sources explica o porquê. Sem chute

![Ranking de quem mais usa](assets/img/screenshots/vyos/web-top.png)

## 🖥️ Sem navegador? Use o terminal

Entre no roteador por SSH e rode um comando só:

```
./vyos-sflow tui
```

A mesma tela, no seu terminal. Aperte q para sair.
Nada é instalado, ótimo para uma olhada rápida.

Extras pequenos: a interface web fala 15 idiomas e tem cinco temas, do claro ao escuro.

## ⚠️ Os limites, com honestidade

- Só sFlow. Nada de NetFlow ou IPFIX
- O histórico é de 48 horas. Ele responde "o que está acontecendo" e "o que acabou de acontecer", não é relatório de longo prazo
- Só Linux amd64. Cobre VyOS e servidores Linux normais, mas não tem versão para Windows ou macOS

Se esses três pontos não te incomodam, ele faz o trabalho muito bem.

## 🔗 Baixe aqui

Grátis e open source, Apache-2.0.
Procure vyos-sflow no GitHub, ou use o link de download deste site.
Não gostou? Apague a pasta e acabou — o roteador em si não é tocado.

Se o VyOS é a saída do seu escritório ou o gateway de uma lan house, experimente.
Aquela sensação de "para onde foi meu tráfego?" some.

E você, usa o quê para olhar o tráfego da sua saída? Conta aí embaixo 👇

#VyOS #sFlow #opensource #redes #sysadmin
